package routing

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMalformedPayloadDoesNotClobber(t *testing.T) {
	h := NewSnapshotHolder()
	good := &RoutingSnapshot{
		Epoch:   5,
		BuiltAt: time.Now(),
		Workers: []WorkerView{{ID: "w1", Endpoint: "e", BaseModel: "m", Healthy: true, Ready: true}},
	}
	h.Store(good)

	var bad RoutingSnapshot
	if err := json.Unmarshal([]byte(`not-json`), &bad); err == nil {
		t.Fatal("expected unmarshal error")
	}
	// simulate subscriber skip: leave holder untouched
	if h.Load().Epoch != 5 || h.Load().Workers[0].ID != "w1" {
		t.Fatalf("clobbered: %+v", h.Load())
	}
}

func TestJSONRoundTrip(t *testing.T) {
	in := &RoutingSnapshot{
		BuiltAt: time.Date(2026, 6, 15, 10, 30, 0, 512e6, time.UTC),
		Epoch:   84213,
		Workers: []WorkerView{
			{ID: "w-ollama-01", Endpoint: "127.0.0.1:50051", BaseModel: "qwen3:8b", Healthy: true, Ready: true, QueueDepth: 2, InFlight: 1},
		},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out RoutingSnapshot
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Epoch != 84213 || len(out.Workers) != 1 || out.Workers[0].QueueDepth != 2 {
		t.Fatalf("%+v", out)
	}
	if len(b) > 4096 {
		t.Fatalf("payload too large: %d", len(b))
	}
}
