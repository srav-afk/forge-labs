package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srav-afk/forge-labs/worker/adapters"
)

func TestOpenAICompatStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	b := NewOpenAICompat(OpenAIConfig{
		ID: "fw", BaseURL: srv.URL, APIKey: "k",
		Models: map[string]string{"qwen3.6:27b": "accounts/x/models/qwen"},
	})
	var texts []string
	err := b.Generate(context.Background(), adapters.GenerateRequest{
		Model: "qwen3.6:27b", Prompt: "hello",
	}, func(c adapters.TokenChunk) error {
		if c.Text != "" {
			texts = append(texts, c.Text)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) == 0 || texts[0] != "hi" {
		t.Fatalf("%v", texts)
	}
}
