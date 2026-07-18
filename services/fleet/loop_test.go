package fleet

import (
	"context"
	"testing"
	"time"

	"github.com/srav-afk/forge-labs/services/routing"
)

func TestCeilDivAndClamp(t *testing.T) {
	if ceilDiv(22, 16) != 2 {
		t.Fatal(ceilDiv(22, 16))
	}
	if clamp(5, 0, 3) != 3 {
		t.Fatal(clamp(5, 0, 3))
	}
}

func TestHysteresisStepsOne(t *testing.T) {
	h := NewHysteresis()
	p := DefaultPolicy(ModelIdentity{BaseModel: "m"})
	p.ScaleDownDelaySeconds = 0
	p.StabilizationWindowSeconds = 1
	now := time.Now()
	got := h.Apply("m", 3, 1, p, now)
	if got != 2 {
		t.Fatalf("want step +1 -> 2, got %d", got)
	}
	h.ClearPending("m")
	got = h.Apply("m", 0, 2, p, now.Add(2*time.Second))
	if got != 1 {
		t.Fatalf("want step -1 -> 1, got %d", got)
	}
}

func TestScaleUpOnLoad(t *testing.T) {
	holder := routing.NewSnapshotHolder()
	holder.Store(&routing.RoutingSnapshot{
		Workers: []routing.WorkerView{
			{ID: "w1", BaseModel: "qwen3.6:27b", Healthy: true, Ready: true, QueueDepth: 20, InFlight: 5},
		},
	})
	pol := NewPolicyCache(nil)
	_ = pol.Upsert(context.Background(), ScalingPolicy{
		BaseModel: "qwen3.6:27b", MinReplicas: 1, MaxReplicas: 3, TargetConcurrency: 16,
		ScaleUpUtilization: 0.7, ScaleDownDelaySeconds: 0, StabilizationWindowSeconds: 1,
	})
	prov := NewLocalProcess()
	metrics := NewMetrics(nil)
	mgr := NewManager(pol, prov, holder, nil, metrics)
	if err := mgr.reconcile(context.Background(), ModelIdentity{BaseModel: "qwen3.6:27b"}); err != nil {
		t.Fatal(err)
	}
	if len(prov.Active()) < 1 {
		t.Fatalf("expected provision, active=%d", len(prov.Active()))
	}
}
