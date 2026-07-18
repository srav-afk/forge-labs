package fleet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLifecycleHTTPModelsReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "Qwen/Qwen2.5-0.5B-Instruct"}},
		})
	}))
	defer srv.Close()

	lc := NewLifecycle(&stubProv{}, nil, nil, LifecycleConfig{
		ReadyTimeout: time.Second,
		PollInterval: 10 * time.Millisecond,
		HTTPClient:   srv.Client(),
	})
	ok, err := lc.httpModelsOK(context.Background(), srv.URL, "Qwen/Qwen2.5-0.5B-Instruct")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

type stubProv struct{}

func (stubProv) Kind() string                                          { return "stub" }
func (stubProv) Provision(context.Context, ModelIdentity) (WorkerID, error) {
	return "runpod-abc", nil
}
func (stubProv) Retire(context.Context, WorkerID) error { return nil }
