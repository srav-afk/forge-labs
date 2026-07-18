package fleet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunPodClientCreateDelete(t *testing.T) {
	var created, deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/pods":
			created = true
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "pod-xyz", "name": "forge-test"})
		case r.Method == http.MethodDelete && r.URL.Path == "/pods/pod-xyz":
			deleted = true
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewRunPodClient("test-key")
	c.base = srv.URL
	c.http = srv.Client()

	pod, err := c.CreatePod(context.Background(), runPodCreateRequest{
		Name:       "forge-test",
		ImageName:  "vllm/vllm-openai:latest",
		GpuTypeIds: []string{"NVIDIA GeForce RTX 4090"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pod.ID != "pod-xyz" {
		t.Fatalf("id=%s", pod.ID)
	}
	if err := c.DeletePod(context.Background(), pod.ID); err != nil {
		t.Fatal(err)
	}
	if !created || !deleted {
		t.Fatalf("created=%v deleted=%v", created, deleted)
	}
	if got := c.ProxyURL("pod-xyz", 8000); got != "https://pod-xyz-8000.proxy.runpod.net" {
		t.Fatalf("proxy=%s", got)
	}
}
