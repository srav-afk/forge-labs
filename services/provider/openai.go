package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/srav-afk/forge-labs/worker/adapters"
)

type OpenAICompat struct {
	id       string
	baseURL  string
	apiKey   string
	modelMap map[string]string
	client   *resty.Client
}

type OpenAIConfig struct {
	ID      string
	BaseURL string
	APIKey  string
	Models  map[string]string
}

func NewOpenAICompat(cfg OpenAIConfig) *OpenAICompat {
	client := resty.New().
		SetBaseURL(strings.TrimRight(cfg.BaseURL, "/")).
		SetHeader("Content-Type", "application/json").
		SetTimeout(0)
	if cfg.APIKey != "" {
		client.SetHeader("Authorization", "Bearer "+cfg.APIKey)
	}
	return &OpenAICompat{
		id:       cfg.ID,
		baseURL:  strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:   cfg.APIKey,
		modelMap: cfg.Models,
		client:   client,
	}
}

func ResolveAPIKey(ref string) string {
	if ref == "" {
		return ""
	}
	if v := os.Getenv(ref); v != "" {
		return v
	}
	if strings.HasPrefix(ref, "FORGE_") {
		return os.Getenv(ref)
	}
	return os.Getenv("FORGE_" + strings.ToUpper(ref))
}

func (b *OpenAICompat) ID() string { return b.id }

func (b *OpenAICompat) ResolveModel(base string) string {
	if m, ok := b.modelMap[base]; ok && m != "" {
		return m
	}
	return base
}

func (b *OpenAICompat) Generate(ctx context.Context, req adapters.GenerateRequest, sink func(adapters.TokenChunk) error) error {
	model := b.ResolveModel(req.Model)
	if model == "" {
		return adapters.ModelNotFound(req.Model)
	}
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": req.Prompt},
		},
		"stream": true,
	}
	if req.Options != nil {
		if v, ok := req.Options["temperature"].(float64); ok {
			body["temperature"] = v
		}
		if v, ok := req.Options["top_p"].(float64); ok {
			body["top_p"] = v
		}
		if v, ok := req.Options["num_predict"].(int); ok && v > 0 {
			body["max_tokens"] = v
		}
	}

	resp, err := b.client.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		SetBody(body).
		Post("/chat/completions")
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("provider %s: %w", b.id, err)
	}
	defer resp.RawBody().Close()

	if resp.StatusCode() == http.StatusNotFound {
		return adapters.ModelNotFound(req.Model)
	}
	if resp.StatusCode() == http.StatusUnauthorized || resp.StatusCode() == http.StatusForbidden {
		return fmt.Errorf("provider %s: auth failed (%d)", b.id, resp.StatusCode())
	}
	if resp.StatusCode() != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.RawBody(), 2048))
		return fmt.Errorf("provider %s: status %d: %s", b.id, resp.StatusCode(), strings.TrimSpace(string(msg)))
	}

	scanner := bufio.NewScanner(resp.RawBody())
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var promptTok, evalTok int32
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			return sink(adapters.TokenChunk{Done: true, FinishReason: "stop", PromptTokens: promptTok, EvalTokens: evalTok})
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int32 `json:"prompt_tokens"`
				CompletionTokens int32 `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			promptTok = chunk.Usage.PromptTokens
			evalTok = chunk.Usage.CompletionTokens
		}
		text := ""
		finish := ""
		if len(chunk.Choices) > 0 {
			text = chunk.Choices[0].Delta.Content
			if chunk.Choices[0].FinishReason != nil {
				finish = *chunk.Choices[0].FinishReason
			}
		}
		if text == "" && finish == "" {
			continue
		}
		out := adapters.TokenChunk{Text: text}
		if finish != "" {
			out.Done = true
			out.FinishReason = finish
			out.PromptTokens = promptTok
			out.EvalTokens = evalTok
		}
		if err := sink(out); err != nil {
			return err
		}
		if out.Done {
			return nil
		}
	}
	return sink(adapters.TokenChunk{Done: true, FinishReason: "stop", PromptTokens: promptTok, EvalTokens: evalTok})
}

func (b *OpenAICompat) Ready(ctx context.Context) bool {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	resp, err := b.client.R().SetContext(cctx).Get("/models")
	if err != nil {
		return b.apiKey != ""
	}
	return resp.StatusCode() < 500
}
