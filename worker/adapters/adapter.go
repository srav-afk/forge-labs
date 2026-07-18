package adapters

import "context"

type GenerateRequest struct {
	Model     string
	Prompt    string
	Options   map[string]any
	KeepAlive string
}

type TokenChunk struct {
	Text         string
	Done         bool
	FinishReason string
	PromptTokens int32
	EvalTokens   int32
	TotalDurNs   int64
}

type Capabilities struct {
	Runtime    string
	Models     []ModelInfo
	Attributes map[string]string
}

type ModelInfo struct {
	BaseModel  string
	Adapter    string
	MaxContext uint32
}

type RuntimeAdapter interface {
	Generate(ctx context.Context, req GenerateRequest, sink func(TokenChunk) error) error
	Capabilities(ctx context.Context) (Capabilities, error)
	Ready(ctx context.Context) bool
}
