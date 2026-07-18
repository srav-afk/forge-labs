package vllm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srav-afk/forge-labs/worker/adapters"
)

func TestGenerateStreamsSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		chunks := []string{
			`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
			`data: {"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`,
			`data: [DONE]`,
		}
		for _, c := range chunks {
			_, _ = io.WriteString(w, c+"\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	a := New(Config{
		BaseURL:     srv.URL,
		ServedModel: "Qwen/Qwen3-235B-A22B",
		ForgeModel:  "qwen3-235b-a22b",
	})
	var got []adapters.TokenChunk
	err := a.Generate(context.Background(), adapters.GenerateRequest{
		Model:  "qwen3-235b-a22b",
		Prompt: "hi",
	}, func(c adapters.TokenChunk) error {
		got = append(got, c)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 2 {
		t.Fatalf("chunks=%d", len(got))
	}
	if got[0].Text != "Hello" {
		t.Fatalf("got0=%+v", got[0])
	}
	last := got[len(got)-1]
	if !last.Done || last.FinishReason == "" {
		t.Fatalf("last=%+v", last)
	}
}

func TestCapabilitiesNormalizesModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"id":"Qwen/Qwen3-235B-A22B"}]}`)
	}))
	defer srv.Close()

	a := New(Config{
		BaseURL:     srv.URL,
		ServedModel: "Qwen/Qwen3-235B-A22B",
		ForgeModel:  "qwen3-235b-a22b",
	})
	caps, err := a.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if caps.Runtime != "vllm" || len(caps.Models) != 1 || caps.Models[0].BaseModel != "qwen3-235b-a22b" {
		t.Fatalf("%+v", caps)
	}
}

func TestScrapeMetrics(t *testing.T) {
	body := `
# HELP vllm:num_requests_running Running
vllm:num_requests_running{model="m"} 3.0
vllm:num_requests_waiting{model="m"} 1.0
vllm:kv_cache_usage_perc{model="m"} 0.4
vllm:gpu_cache_usage_perc{model="m"} 0.55
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	a := New(Config{BaseURL: srv.URL})
	snap, err := a.ScrapeMetrics(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Running != 3 || snap.Waiting != 1 || snap.KVCacheUsage != 0.4 {
		t.Fatalf("%+v", snap)
	}
	if a.LastLoad().Running != 3 {
		t.Fatal("last load not stored")
	}
}

func TestGenerateModelNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"message":"model not found"}}`)
	}))
	defer srv.Close()

	a := New(Config{BaseURL: srv.URL, ServedModel: "x"})
	err := a.Generate(context.Background(), adapters.GenerateRequest{
		Model: "x", Prompt: "hi",
	}, func(adapters.TokenChunk) error { return nil })
	var nf *adapters.ModelNotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err=%v", err)
	}
}

func TestParsePrometheusUnderscoreNames(t *testing.T) {
	snap := parsePrometheus("vllm_num_requests_running 2\nvllm_kv_cache_usage_perc 0.2\n")
	if snap.Running != 2 || snap.KVCacheUsage != 0.2 {
		t.Fatalf("%+v", snap)
	}
}
