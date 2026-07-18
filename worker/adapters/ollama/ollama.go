package ollama

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/srav-afk/forge-labs/worker/adapters"
)

type Adapter struct {
	baseURL   string
	keepAlive string
	client    *resty.Client
}

type Config struct {
	BaseURL   string
	KeepAlive string
	Timeout   time.Duration
}

func New(cfg Config) *Adapter {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://127.0.0.1:11434"
	}
	if cfg.KeepAlive == "" {
		cfg.KeepAlive = "5m"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 0
	}
	client := resty.New().
		SetBaseURL(strings.TrimRight(cfg.BaseURL, "/")).
		SetHeader("Content-Type", "application/json")
	if cfg.Timeout > 0 {
		client.SetTimeout(cfg.Timeout)
	}
	return &Adapter{
		baseURL:   strings.TrimRight(cfg.BaseURL, "/"),
		keepAlive: cfg.KeepAlive,
		client:    client,
	}
}

type generateBody struct {
	Model     string         `json:"model"`
	Prompt    string         `json:"prompt"`
	Stream    bool           `json:"stream"`
	KeepAlive string         `json:"keep_alive,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
}

type streamLine struct {
	Model           string `json:"model"`
	Response        string `json:"response"`
	Done            bool   `json:"done"`
	DoneReason      string `json:"done_reason"`
	PromptEvalCount int32  `json:"prompt_eval_count"`
	EvalCount       int32  `json:"eval_count"`
	TotalDuration   int64  `json:"total_duration"`
	Error           string `json:"error"`
}

type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

type errorBody struct {
	Error string `json:"error"`
}

func (a *Adapter) Generate(ctx context.Context, req adapters.GenerateRequest, sink func(adapters.TokenChunk) error) error {
	if req.Model == "" {
		return adapters.ModelNotFound("")
	}
	if req.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}

	keepAlive := req.KeepAlive
	if keepAlive == "" {
		keepAlive = a.keepAlive
	}

	body := generateBody{
		Model:     req.Model,
		Prompt:    req.Prompt,
		Stream:    true,
		KeepAlive: keepAlive,
		Options:   req.Options,
	}

	resp, err := a.client.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		SetBody(body).
		Post("/api/generate")
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ollama generate: %w", err)
	}
	defer resp.RawBody().Close()

	if resp.StatusCode() == http.StatusNotFound {
		return adapters.ModelNotFound(req.Model)
	}
	if resp.StatusCode() != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.RawBody(), 4096))
		var eb errorBody
		if json.Unmarshal(msg, &eb) == nil && eb.Error != "" {
			if isModelMissing(eb.Error) {
				return adapters.ModelNotFound(req.Model)
			}
			return fmt.Errorf("ollama generate: %s", eb.Error)
		}
		return fmt.Errorf("ollama generate: status %d: %s", resp.StatusCode(), strings.TrimSpace(string(msg)))
	}

	scanner := bufio.NewScanner(resp.RawBody())
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var chunk streamLine
		if err := json.Unmarshal(line, &chunk); err != nil {
			return fmt.Errorf("ollama ndjson: %w", err)
		}
		if chunk.Error != "" {
			if isModelMissing(chunk.Error) {
				return adapters.ModelNotFound(req.Model)
			}
			return fmt.Errorf("ollama generate: %s", chunk.Error)
		}
		out := adapters.TokenChunk{
			Text: chunk.Response,
			Done: chunk.Done,
		}
		if chunk.Done {
			out.FinishReason = chunk.DoneReason
			if out.FinishReason == "" {
				out.FinishReason = "stop"
			}
			out.PromptTokens = chunk.PromptEvalCount
			out.EvalTokens = chunk.EvalCount
			out.TotalDurNs = chunk.TotalDuration
		}
		if err := sink(out); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ollama stream: %w", err)
	}
	return nil
}

func (a *Adapter) Capabilities(ctx context.Context) (adapters.Capabilities, error) {
	var tags tagsResponse
	resp, err := a.client.R().
		SetContext(ctx).
		SetResult(&tags).
		Get("/api/tags")
	if err != nil {
		return adapters.Capabilities{}, fmt.Errorf("ollama tags: %w", err)
	}
	if resp.IsError() {
		return adapters.Capabilities{}, fmt.Errorf("ollama tags: status %d", resp.StatusCode())
	}

	caps := adapters.Capabilities{
		Runtime:    "ollama",
		Attributes: map[string]string{},
		Models:     make([]adapters.ModelInfo, 0, len(tags.Models)),
	}
	for _, m := range tags.Models {
		caps.Models = append(caps.Models, adapters.ModelInfo{BaseModel: m.Name})
	}
	return caps, nil
}

func (a *Adapter) Ready(ctx context.Context) bool {
	resp, err := a.client.R().
		SetContext(ctx).
		Get("/api/tags")
	if err != nil {
		return false
	}
	return resp.StatusCode() == http.StatusOK
}

func (a *Adapter) HasModel(ctx context.Context, model string) (bool, error) {
	caps, err := a.Capabilities(ctx)
	if err != nil {
		return false, err
	}
	for _, m := range caps.Models {
		if m.BaseModel == model || strings.HasPrefix(m.BaseModel, model+":") {
			return true, nil
		}
	}
	return false, nil
}

func isModelMissing(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "not found") || strings.Contains(lower, "pull")
}
