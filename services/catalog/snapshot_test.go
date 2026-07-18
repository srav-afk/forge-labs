package catalog

import (
	"testing"
	"time"
)

func TestSnapshotResolve(t *testing.T) {
	s := &Snapshot{
		BuiltAt: time.Now(),
		ByName: map[string]ModelIdentity{
			"qwen3:8b": {ID: "m2", Name: "qwen3:8b", BaseModel: "qwen3:8b"},
			"support-bot": {
				ID: "m3", Name: "support-bot", BaseModel: "qwen3:8b", Adapter: "support-bot-v3",
			},
		},
		WorkersByModel: map[string][]string{
			"m2": {"workerB"},
			"m3": {"workerA"},
		},
	}

	id, workers, ok := s.Resolve("qwen3:8b")
	if !ok || id.ID != "m2" || len(workers) != 1 || workers[0] != "workerB" {
		t.Fatalf("qwen: id=%+v workers=%v ok=%v", id, workers, ok)
	}
	id, workers, ok = s.Resolve("support-bot")
	if !ok || id.BaseModel != "qwen3:8b" || id.Adapter != "support-bot-v3" || workers[0] != "workerA" {
		t.Fatalf("lora: id=%+v workers=%v", id, workers)
	}
	if _, _, ok := s.Resolve("missing"); ok {
		t.Fatal("missing should not resolve")
	}
}

func TestSnapshotHolderAtomicSwap(t *testing.T) {
	h := NewSnapshotHolder()
	if h.Load() != nil {
		t.Fatal("expected nil")
	}
	h.Store(&Snapshot{
		ByName: map[string]ModelIdentity{"a": {ID: "1", Name: "a", BaseModel: "a"}},
	})
	if h.Load().Empty() {
		t.Fatal("expected non-empty")
	}
	h.Store(&Snapshot{
		ByName: map[string]ModelIdentity{
			"a": {ID: "1", Name: "a", BaseModel: "a"},
			"b": {ID: "2", Name: "b", BaseModel: "b"},
		},
	})
	if len(h.Load().ByName) != 2 {
		t.Fatalf("got %d", len(h.Load().ByName))
	}
}

func TestSnapshotEmptyAndNames(t *testing.T) {
	var s *Snapshot
	if !s.Empty() {
		t.Fatal("nil not empty")
	}
	s = &Snapshot{ByName: map[string]ModelIdentity{}}
	if !s.Empty() {
		t.Fatal("empty map not empty")
	}
	s.ByName["x"] = ModelIdentity{ID: "1", Name: "x", BaseModel: "x"}
	names := s.ModelNames()
	if len(names) != 1 || names[0].Name != "x" {
		t.Fatalf("%+v", names)
	}
}
