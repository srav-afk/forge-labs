package gateway

import (
	"errors"
	"testing"
	"time"

	"github.com/srav-afk/forge-labs/services/catalog"
	"github.com/srav-afk/forge-labs/services/routing"
	"github.com/srav-afk/forge-labs/services/scheduler"
)

func newTestSelector(h *routing.SnapshotHolder) *SnapshotSelector {
	return NewSnapshotSelector(h, catalog.NewSnapshotHolder(), routing.NewInflightTracker(), scheduler.NewLatencyStore(10*time.Second, nil), scheduler.DefaultChain(), nil, 4)
}

func newTestSelectorWithCatalog(h *routing.SnapshotHolder, cat *catalog.SnapshotHolder) *SnapshotSelector {
	return NewSnapshotSelector(h, cat, routing.NewInflightTracker(), scheduler.NewLatencyStore(10*time.Second, nil), scheduler.DefaultChain(), nil, 4)
}

func TestSnapshotSelectorNoSnapshot(t *testing.T) {
	s := newTestSelector(routing.NewSnapshotHolder())
	_, err := s.SelectWorker("qwen3:8b", "hi")
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
	s := newTestSelector(h)
	w, err := s.SelectWorker("qwen3:8b", "hi")
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

func TestSnapshotSelectorLeastLoaded(t *testing.T) {
	h := routing.NewSnapshotHolder()
	h.Store(&routing.RoutingSnapshot{
		Epoch: 1,
		Workers: []routing.WorkerView{
			{ID: "busy", Endpoint: "b", BaseModel: "m", Healthy: true, Ready: true, InFlight: 4},
			{ID: "idle", Endpoint: "i", BaseModel: "m", Healthy: true, Ready: true, InFlight: 0},
		},
	})
	s := newTestSelector(h)
	w, err := s.SelectWorker("m", "system: shared")
	if err != nil {
		t.Fatal(err)
	}
	if w.ID != "idle" {
		t.Fatalf("got %s", w.ID)
	}
}

func TestSnapshotSelectorPrefersLowerEWMA(t *testing.T) {
	h := routing.NewSnapshotHolder()
	h.Store(&routing.RoutingSnapshot{
		Epoch: 1,
		Workers: []routing.WorkerView{
			{ID: "slow", Endpoint: "s", BaseModel: "m", Healthy: true, Ready: true},
			{ID: "fast", Endpoint: "f", BaseModel: "m", Healthy: true, Ready: true},
		},
	})
	store := scheduler.NewLatencyStore(10*time.Second, nil)
	store.Observe("slow", 400)
	store.Observe("fast", 40)
	chain := scheduler.NewConfiguredChain(scheduler.ChainConfig{
		WeightLoad:     0.8,
		WeightLatency:  0.2,
		WeightAffinity: 0,
		LatencyRefMs:   100,
		AdmissionLimit: 4,
	})
	s := NewSnapshotSelector(h, catalog.NewSnapshotHolder(), routing.NewInflightTracker(), store, chain, nil, 4)
	w, err := s.SelectWorker("m", "")
	if err != nil {
		t.Fatal(err)
	}
	if w.ID != "fast" {
		t.Fatalf("got %s want fast", w.ID)
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
	s := newTestSelector(h)
	for i := 0; i < 1000; i++ {
		if _, err := s.SelectWorker("m", "system: shared"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSnapshotSelectorNoCapacity(t *testing.T) {
	h := routing.NewSnapshotHolder()
	h.Store(&routing.RoutingSnapshot{
		Epoch: 1,
		Workers: []routing.WorkerView{
			{ID: "w1", Endpoint: "e", BaseModel: "other", Healthy: true, Ready: true},
		},
	})
	s := newTestSelector(h)
	_, err := s.SelectWorker("missing", "x")
	if !errors.Is(err, scheduler.ErrNoCapacity) {
		t.Fatalf("err=%v", err)
	}
}

func TestCatalogRoutesByAssignment(t *testing.T) {
	h := routing.NewSnapshotHolder()
	h.Store(&routing.RoutingSnapshot{
		Epoch: 1,
		Workers: []routing.WorkerView{
			{ID: "workerA", Endpoint: "a:1", BaseModel: "llama-3.1-8b", Healthy: true, Ready: true},
			{ID: "workerB", Endpoint: "b:1", BaseModel: "qwen2.5-7b", Healthy: true, Ready: true},
		},
	})
	cat := catalog.NewSnapshotHolder()
	cat.Store(&catalog.Snapshot{
		BuiltAt: time.Now(),
		ByName: map[string]catalog.ModelIdentity{
			"llama-3.1-8b": {ID: "m1", Name: "llama-3.1-8b", BaseModel: "llama-3.1-8b"},
			"qwen2.5-7b":   {ID: "m2", Name: "qwen2.5-7b", BaseModel: "qwen2.5-7b"},
		},
		WorkersByModel: map[string][]string{
			"m1": {"workerA"},
			"m2": {"workerB"},
		},
	})
	s := newTestSelectorWithCatalog(h, cat)

	w, err := s.SelectWorker("qwen2.5-7b", "hi")
	if err != nil {
		t.Fatal(err)
	}
	if w.ID != "workerB" {
		t.Fatalf("qwen -> %s want workerB", w.ID)
	}
	w, err = s.SelectWorker("llama-3.1-8b", "hi")
	if err != nil {
		t.Fatal(err)
	}
	if w.ID != "workerA" {
		t.Fatalf("llama -> %s want workerA", w.ID)
	}
}

func TestCatalogUnknownModel404(t *testing.T) {
	h := routing.NewSnapshotHolder()
	h.Store(&routing.RoutingSnapshot{
		Epoch: 1,
		Workers: []routing.WorkerView{
			{ID: "w1", Endpoint: "e", BaseModel: "m", Healthy: true, Ready: true},
		},
	})
	cat := catalog.NewSnapshotHolder()
	cat.Store(&catalog.Snapshot{
		BuiltAt: time.Now(),
		ByName: map[string]catalog.ModelIdentity{
			"known": {ID: "m1", Name: "known", BaseModel: "known"},
		},
		WorkersByModel: map[string][]string{"m1": {"w1"}},
	})
	s := newTestSelectorWithCatalog(h, cat)
	_, err := s.SelectWorker("does-not-exist", "x")
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("err=%v", err)
	}
	status, typ, code := selectErrorStatus(err)
	if status != 404 || typ != "invalid_request_error" || code != "model_not_found" {
		t.Fatalf("status=%d type=%s code=%s", status, typ, code)
	}
}

func TestCatalogSkipsAssignedButDown(t *testing.T) {
	h := routing.NewSnapshotHolder()
	h.Store(&routing.RoutingSnapshot{
		Epoch: 1,
		Workers: []routing.WorkerView{
			{ID: "workerB", Endpoint: "b:1", BaseModel: "qwen2.5-7b", Healthy: false, Ready: false},
			{ID: "workerA", Endpoint: "a:1", BaseModel: "qwen2.5-7b", Healthy: true, Ready: true},
		},
	})
	cat := catalog.NewSnapshotHolder()
	cat.Store(&catalog.Snapshot{
		BuiltAt: time.Now(),
		ByName: map[string]catalog.ModelIdentity{
			"qwen2.5-7b": {ID: "m2", Name: "qwen2.5-7b", BaseModel: "qwen2.5-7b"},
		},
		WorkersByModel: map[string][]string{"m2": {"workerB"}},
	})
	s := newTestSelectorWithCatalog(h, cat)
	_, err := s.SelectWorker("qwen2.5-7b", "hi")
	if !errors.Is(err, scheduler.ErrNoCapacity) && !errors.Is(err, ErrNoLiveAssignee) {
		t.Fatalf("err=%v want no live capacity", err)
	}
}

func TestCatalogListModelsPrefersCatalog(t *testing.T) {
	h := routing.NewSnapshotHolder()
	h.Store(&routing.RoutingSnapshot{
		Epoch: 1,
		Workers: []routing.WorkerView{
			{ID: "w1", Endpoint: "e", BaseModel: "from-worker", Healthy: true, Ready: true},
		},
	})
	cat := catalog.NewSnapshotHolder()
	cat.Store(&catalog.Snapshot{
		BuiltAt: time.Unix(1700000000, 0),
		ByName: map[string]catalog.ModelIdentity{
			"catalog-model": {ID: "m1", Name: "catalog-model", BaseModel: "catalog-model"},
		},
		WorkersByModel: map[string][]string{"m1": {"w1"}},
	})
	s := newTestSelectorWithCatalog(h, cat)
	models := s.ListModels()
	if len(models) != 1 || models[0].ID != "catalog-model" {
		t.Fatalf("%+v", models)
	}
}

func TestCatalogIntersectLivePicksHealthyReplica(t *testing.T) {
	h := routing.NewSnapshotHolder()
	h.Store(&routing.RoutingSnapshot{
		Epoch: 1,
		Workers: []routing.WorkerView{
			{ID: "dead", Endpoint: "d", BaseModel: "m", Healthy: false, Ready: false},
			{ID: "live", Endpoint: "l", BaseModel: "m", Healthy: true, Ready: true},
		},
	})
	cat := catalog.NewSnapshotHolder()
	cat.Store(&catalog.Snapshot{
		BuiltAt: time.Now(),
		ByName: map[string]catalog.ModelIdentity{
			"m": {ID: "mid", Name: "m", BaseModel: "m"},
		},
		WorkersByModel: map[string][]string{"mid": {"dead", "live"}},
	})
	s := newTestSelectorWithCatalog(h, cat)
	w, err := s.SelectWorker("m", "hi")
	if err != nil {
		t.Fatal(err)
	}
	if w.ID != "live" {
		t.Fatalf("got %s", w.ID)
	}
}
