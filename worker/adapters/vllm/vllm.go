package vllm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/srav-afk/forge-labs/worker/adapters"
)

type Adapter struct {
	baseURL     string
	servedModel string
	forgeModel  string
	client      *resty.Client

	mu   sync.RWMutex
	load LoadSnapshot
}

type Config struct {
	BaseURL     string
	ServedModel string
	ForgeModel  string
	Timeout     time.Duration
}

type LoadSnapshot struct {
	Running       float64
	Waiting       float64
	KVCacheUsage  float64
	GPUCacheUsage float64
	UpdatedAt     time.Time
}

func New(cfg Config) *Adapter {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://127.0.0.1:8000"
	}
	client := resty.New().
		SetBaseURL(strings.TrimRight(cfg.BaseURL, "/")).
		SetHeader("Content-Type", "application/json")
	if cfg.Timeout > 0 {
		client.SetTimeout(cfg.Timeout)
	}
	return &Adapter{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		servedModel: cfg.ServedModel,
		forgeModel:  cfg.ForgeModel,
		client:      client,
	}
}

type chatBody struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature *float64      `json:"temperature,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type sseChoice struct {
	Delta struct {
		Content string `json:"content"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

type sseChunk struct {
	Choices []sseChoice `json:"choices"`
	Usage   *struct {
		PromptTokens     int32 `json:"prompt_tokens"`
		CompletionTokens int32 `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func (a *Adapter) resolveServed(model string) string {
	if a.servedModel != "" {
		if model == "" || model == a.forgeModel || model == a.servedModel {
			return a.servedModel
		}
	}
	if model != "" {
		return model
	}
	if a.forgeModel != "" {
		return a.forgeModel
	}
	return a.servedModel
}

func (a *Adapter) toForge(served string) string {
	if a.forgeModel != "" && (served == a.servedModel || served == a.forgeModel || a.servedModel == "") {
		return a.forgeModel
	}
	return served
}

func (a *Adapter) Generate(ctx context.Context, req adapters.GenerateRequest, sink func(adapters.TokenChunk) error) error {
	if req.Model == "" && a.forgeModel == "" && a.servedModel == "" {
		return adapters.ModelNotFound("")
	}
	if req.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}

	body := chatBody{
		Model: a.resolveServed(req.Model),
		Messages: []chatMessage{
			{Role: "user", Content: req.Prompt},
		},
		Stream: true,
	}
	if req.Options != nil {
		if v, ok := req.Options["temperature"].(float64); ok {
			body.Temperature = &v
		}
		if v, ok := req.Options["top_p"].(float64); ok {
			body.TopP = &v
		}
		if v, ok := req.Options["num_predict"].(int); ok && v > 0 {
			body.MaxTokens = &v
		}
		if v, ok := req.Options["max_tokens"].(int); ok && v > 0 {
			body.MaxTokens = &v
		}
	}

	resp, err := a.client.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		SetBody(body).
		Post("/v1/chat/completions")
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("vllm generate: %w", err)
	}
	defer resp.RawBody().Close()

	if resp.StatusCode() == http.StatusNotFound {
		return adapters.ModelNotFound(req.Model)
	}
	if resp.StatusCode() != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.RawBody(), 4096))
		if isModelMissing(string(msg)) {
			return adapters.ModelNotFound(req.Model)
		}
		return fmt.Errorf("vllm generate: status %d: %s", resp.StatusCode(), strings.TrimSpace(string(msg)))
	}

	scanner := bufio.NewScanner(resp.RawBody())
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var promptTok, evalTok int32
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			return sink(adapters.TokenChunk{
				Done:         true,
				FinishReason: "stop",
				PromptTokens: promptTok,
				EvalTokens:   evalTok,
			})
		}
		var chunk sseChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("vllm sse: %w", err)
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			if isModelMissing(chunk.Error.Message) {
				return adapters.ModelNotFound(req.Model)
			}
			return fmt.Errorf("vllm generate: %s", chunk.Error.Message)
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
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("vllm stream: %w", err)
	}
	return sink(adapters.TokenChunk{
		Done:         true,
		FinishReason: "stop",
		PromptTokens: promptTok,
		EvalTokens:   evalTok,
	})
}

func (a *Adapter) Capabilities(ctx context.Context) (adapters.Capabilities, error) {
	var body modelsResponse
	resp, err := a.client.R().
		SetContext(ctx).
		SetResult(&body).
		Get("/v1/models")
	if err != nil {
		return adapters.Capabilities{}, fmt.Errorf("vllm models: %w", err)
	}
	if resp.IsError() {
		return adapters.Capabilities{}, fmt.Errorf("vllm models: status %d", resp.StatusCode())
	}
	caps := adapters.Capabilities{
		Runtime:    "vllm",
		Attributes: map[string]string{},
		Models:     make([]adapters.ModelInfo, 0, len(body.Data)),
	}
	for _, m := range body.Data {
		caps.Models = append(caps.Models, adapters.ModelInfo{BaseModel: a.toForge(m.ID)})
	}
	if len(caps.Models) == 0 && a.forgeModel != "" {
		caps.Models = append(caps.Models, adapters.ModelInfo{BaseModel: a.forgeModel})
	}
	return caps, nil
}

func (a *Adapter) Ready(ctx context.Context) bool {
	resp, err := a.client.R().
		SetContext(ctx).
		Get("/v1/models")
	if err != nil {
		return false
	}
	return resp.StatusCode() == http.StatusOK
}

func (a *Adapter) ScrapeMetrics(ctx context.Context) (LoadSnapshot, error) {
	resp, err := a.client.R().
		SetContext(ctx).
		Get("/metrics")
	if err != nil {
		return LoadSnapshot{}, err
	}
	if resp.IsError() {
		return LoadSnapshot{}, fmt.Errorf("vllm metrics: status %d", resp.StatusCode())
	}
	snap := parsePrometheus(resp.String())
	snap.UpdatedAt = time.Now().UTC()
	a.mu.Lock()
	a.load = snap
	a.mu.Unlock()
	return snap, nil
}

func (a *Adapter) LastLoad() LoadSnapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.load
}

func parsePrometheus(body string) LoadSnapshot {
	var snap LoadSnapshot
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, val, ok := splitMetric(line)
		if !ok {
			continue
		}
		switch {
		case strings.HasPrefix(name, "vllm:num_requests_running") || strings.HasPrefix(name, "vllm_num_requests_running"):
			snap.Running = val
		case strings.HasPrefix(name, "vllm:num_requests_waiting") || strings.HasPrefix(name, "vllm_num_requests_waiting"):
			snap.Waiting = val
		case strings.HasPrefix(name, "vllm:kv_cache_usage_perc") || strings.HasPrefix(name, "vllm_kv_cache_usage_perc"):
			snap.KVCacheUsage = val
		case strings.HasPrefix(name, "vllm:gpu_cache_usage_perc") || strings.HasPrefix(name, "vllm_gpu_cache_usage_perc"):
			snap.GPUCacheUsage = val
		}
	}
	return snap
}

func splitMetric(line string) (name string, val float64, ok bool) {
	// name{labels} value  OR  name value
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", 0, false
	}
	name = fields[0]
	if i := strings.IndexByte(name, '{'); i >= 0 {
		name = name[:i]
	}
	v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
	if err != nil {
		return "", 0, false
	}
	return name, v, true
}

func isModelMissing(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "not found") || strings.Contains(lower, "does not exist")
}
