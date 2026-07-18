package gateway

import (
	"fmt"
	"strings"

	workerv1 "github.com/srav-afk/forge-labs/gen/worker/v1"
)

func ParseModelID(id string) (base, adapter string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", ""
	}
	if i := strings.Index(id, "#"); i >= 0 {
		return id[:i], id[i+1:]
	}
	return id, ""
}

func messagesToPrompt(messages []chatMessage) string {
	var b strings.Builder
	for _, m := range messages {
		role := m.Role
		if role == "" {
			role = "user"
		}
		b.WriteString(role)
		b.WriteString(": ")
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	b.WriteString("assistant: ")
	return b.String()
}

func promptFromCompletions(prompt any) (string, error) {
	switch v := prompt.(type) {
	case string:
		return v, nil
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return "", fmt.Errorf("prompt array must contain strings")
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, "\n"), nil
	default:
		return "", fmt.Errorf("prompt must be a string or array of strings")
	}
}

func toWorkerRequest(model, prompt string, temperature *float32, topP *float32, maxTokens *int32) *workerv1.GenerateRequest {
	base, adapter := ParseModelID(model)
	req := &workerv1.GenerateRequest{
		Model:  &workerv1.ModelRef{BaseModel: base},
		Prompt: prompt,
	}
	if adapter != "" {
		req.Model.Adapter = &adapter
	}
	sampling := &workerv1.SamplingParams{}
	has := false
	if temperature != nil {
		sampling.Temperature = *temperature
		has = true
	}
	if topP != nil {
		sampling.TopP = *topP
		has = true
	}
	if maxTokens != nil {
		sampling.MaxTokens = *maxTokens
		has = true
	}
	if has {
		req.Sampling = sampling
	}
	return req
}

func maxTokensFromChat(req chatCompletionRequest) *int32 {
	if req.MaxCompletionTokens != nil {
		return req.MaxCompletionTokens
	}
	return req.MaxTokens
}
