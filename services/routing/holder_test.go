package routing

import (
	"math"
	"testing"
	"time"

	"github.com/srav-afk/forge-labs/internal/observability"
)

func TestStoreIfNewer(t *testing.T) {
	h := NewSnapshotHolder()
	if h.Load() != nil {
		t.Fatal("expected nil")
	}
	older := &RoutingSnapshot{Epoch: 1, BuiltAt: time.Now().Add(-time.Second)}
	newer := &RoutingSnapshot{Epoch: 2, BuiltAt: time.Now()}
	if !h.StoreIfNewer(older) {
		t.Fatal("first store")
	}
	if h.StoreIfNewer(older) {
		t.Fatal("same epoch should not replace")
	}
	if !h.StoreIfNewer(newer) {
		t.Fatal("newer epoch should replace")
	}
	if h.Load().Epoch != 2 {
		t.Fatalf("epoch=%d", h.Load().Epoch)
	}
	stale := &RoutingSnapshot{Epoch: 1, BuiltAt: time.Now()}
	if h.StoreIfNewer(stale) {
		t.Fatal("older epoch must not win")
	}
}

func TestSnapshotAgeMetric(t *testing.T) {
	h := NewSnapshotHolder()
	reg := observability.NewRegistry()
	_ = NewMetrics(reg, h)

	g := gatherGauge(t, reg, "forge_routing_snapshot_age_seconds")
	if !math.IsInf(g, 1) {
		t.Fatalf("want +Inf got %v", g)
	}

	h.Store(&RoutingSnapshot{Epoch: 1, BuiltAt: time.Now().Add(-2 * time.Second)})
	g = gatherGauge(t, reg, "forge_routing_snapshot_age_seconds")
	if g < 1.5 || g > 5 {
		t.Fatalf("age=%v", g)
	}
}

func gatherGauge(t *testing.T, reg *observability.Registry, name string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if m.Gauge != nil {
				return m.GetGauge().GetValue()
			}
		}
	}
	t.Fatalf("metric %s not found", name)
	return 0
}
