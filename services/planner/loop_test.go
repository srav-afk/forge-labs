package planner

import (
	"testing"
	"time"

	"github.com/srav-afk/forge-labs/services/routing"
)

func TestPlanPrefersCheapWhenCostWeighted(t *testing.T) {
	s := &Service{}
	obj := DefaultObjective()
	obj.WeightLoad = 0.2
	obj.WeightCost = 0.8
	obj.WeightLatency = 0
	snap := &routing.RoutingSnapshot{
		Workers: []routing.WorkerView{
			{ID: "mac", BaseModel: "qwen3.6:27b", Healthy: true, Ready: true, CostPerHour: 0, QueueDepth: 2},
			{ID: "pod", BaseModel: "qwen3.6:27b", Healthy: true, Ready: true, CostPerHour: 2.0, QueueDepth: 0},
		},
	}
	p, _, _ := s.plan(obj, snap)
	mp := p.Models["qwen3.6:27b"]
	if mp.Weights["mac"] <= mp.Weights["pod"] {
		t.Fatalf("mac should win under cost weight: %+v", mp.Weights)
	}
}

func TestPolicyHolderVersionGate(t *testing.T) {
	h := routing.NewPolicyHolder()
	if !h.StoreIfNewer(&routing.RoutingPolicy{Version: 2, WeightLoad: 0.5}) {
		t.Fatal("v2")
	}
	if h.StoreIfNewer(&routing.RoutingPolicy{Version: 1, WeightLoad: 0.9}) {
		t.Fatal("stale should not swap")
	}
	if h.Load().Version != 2 {
		t.Fatalf("got %d", h.Load().Version)
	}
	if !h.StoreIfNewer(&routing.RoutingPolicy{Version: 3}) {
		t.Fatal("v3")
	}
}

func TestHashObjectiveStable(t *testing.T) {
	o := DefaultObjective()
	o.UpdatedAt = time.Now()
	a := HashObjective(o)
	b := HashObjective(o)
	if a != b || a == "" {
		t.Fatalf("%s %s", a, b)
	}
}
