package ollama

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/srav-afk/forge-labs/worker/adapters"
)

func TestGenerateStreamsChunks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		lines := []string{
			`{"response":"The","done":false}`,
			`{"response":" sky","done":false}`,
			`{"response":"","done":true,"done_reason":"stop","prompt_eval_count":11,"eval_count":42,"total_duration":4900000000}`,
		}
		for _, line := range lines {
			_, _ = io.WriteString(w, line+"\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	a := New(Config{BaseURL: srv.URL, KeepAlive: "5m"})
	var got []adapters.TokenChunk
	err := a.Generate(context.Background(), adapters.GenerateRequest{
		Model:  "llama3.2",
		Prompt: "Why is the sky blue?",
	}, func(c adapters.TokenChunk) error {
		got = append(got, c)
		return nil
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("chunks = %d, want 3", len(got))
	}
	if got[0].Text != "The" || got[0].Done {
		t.Fatalf("chunk0 = %+v", got[0])
	}
	if got[1].Text != " sky" {
		t.Fatalf("chunk1 = %+v", got[1])
	}
	if !got[2].Done || got[2].FinishReason != "stop" {
		t.Fatalf("chunk2 = %+v", got[2])
	}
	if got[2].PromptTokens != 11 || got[2].EvalTokens != 42 || got[2].TotalDurNs != 4900000000 {
		t.Fatalf("usage = %+v", got[2])
	}
}

func TestGenerateModelNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"model 'missing' not found"}`)
	}))
	defer srv.Close()

	a := New(Config{BaseURL: srv.URL})
	err := a.Generate(context.Background(), adapters.GenerateRequest{
		Model:  "missing",
		Prompt: "hi",
	}, func(adapters.TokenChunk) error { return nil })
	var nf *adapters.ModelNotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want ModelNotFoundError", err)
	}
}

func TestGenerateCancelsUpstream(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, `{"response":"hi","done":false}`+"\n")
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			_, _ = io.WriteString(w, `{"response":"late","done":true}`+"\n")
		}
	}))
	defer srv.Close()

	a := New(Config{BaseURL: srv.URL})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()

	err := a.Generate(ctx, adapters.GenerateRequest{
		Model:  "llama3.2",
		Prompt: "hi",
	}, func(adapters.TokenChunk) error { return nil })
	if err == nil {
		t.Fatal("expected cancel error")
	}
	if !errors.Is(err, context.Canceled) && !stringsContainsCancel(err) {
		t.Fatalf("err = %v", err)
	}
}

func stringsContainsCancel(err error) bool {
	return err != nil && (errors.Is(err, context.Canceled) || strings.Contains(strings.ToLower(err.Error()), "cancel"))
}
