package gateway

import (
	"errors"
	"testing"
	"time"

	"github.com/srav-afk/forge-labs/services/routing"
)

func TestSnapshotSelectorNoSnapshot(t *testing.T) {
	s := NewSnapshotSelector(routing.NewSnapshotHolder())
	_, err := s.SelectWorker("qwen3:8b")
	if !errors.Is(err, ErrNoSnapshot) {
		t.Fatalf("err=%v", err)
	}
	if models := s.ListModels(); models != nil {
		t.Fatalf("models=%v", models)
	}
}

func TestSnapshotSelectorPicksHealthyReady(t *testing.T) {
	h := routing.NewSnapshotHolder()
	h.Store(&routing.RoutingSnapshot{
		BuiltAt: time.Now(),
		Epoch:   1,
		Workers: []routing.WorkerView{
			{ID: "dead", Endpoint: "127.0.0.1:1", BaseModel: "qwen3:8b", Healthy: false, Ready: false},
			{ID: "live", Endpoint: "127.0.0.1:50051", BaseModel: "qwen3:8b", Healthy: true, Ready: true, QueueDepth: 2},
		},
	})
	s := NewSnapshotSelector(h)
	w, err := s.SelectWorker("qwen3:8b")
	if err != nil {
		t.Fatal(err)
	}
	if w.ID != "live" || w.Endpoint != "127.0.0.1:50051" {
		t.Fatalf("%+v", w)
	}
	models := s.ListModels()
	if len(models) != 1 || models[0].OwnedBy != "forge" {
		t.Fatalf("%+v", models)
	}
}

func TestSnapshotSelectorHotPathIsMemoryOnly(t *testing.T) {
	h := routing.NewSnapshotHolder()
	h.Store(&routing.RoutingSnapshot{
		Epoch: 1,
		Workers: []routing.WorkerView{
			{ID: "w1", Endpoint: "127.0.0.1:50051", BaseModel: "m", Healthy: true, Ready: true},
		},
	})
	s := NewSnapshotSelector(h)
	for i := 0; i < 1000; i++ {
		if _, err := s.SelectWorker("m"); err != nil {
			t.Fatal(err)
		}
	}
}
