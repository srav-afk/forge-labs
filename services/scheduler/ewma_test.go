package scheduler

import (
	"math"
	"testing"
	"time"
)

func TestEWMATimeDecayWorkedExample(t *testing.T) {
	e := NewEWMA(10*time.Second, 0)
	t0 := time.Unix(0, 0)
	e.Update(40, t0)
	if e.Value() != 40 {
		t.Fatalf("seed=%v", e.Value())
	}
	// sample 200ms after 80ms wall clock from last
	e.Update(200, t0.Add(80*time.Millisecond))
	// alpha = 1 - exp(-0.08/10) ≈ 0.007968
	// ewma ≈ 0.007968*200 + 0.992032*40 ≈ 41.274
	got := e.Value()
	if math.Abs(got-41.27) > 0.05 {
		t.Fatalf("ewma=%v want ~41.27", got)
	}
}

func TestLatencyScorerPrefersFaster(t *testing.T) {
	s := NewLatencyScorer(100)
	fast := s.Score(nil, nil, Candidate{EwmaLatencyMs: 20})
	slow := s.Score(nil, nil, Candidate{EwmaLatencyMs: 200})
	unknown := s.Score(nil, nil, Candidate{EwmaLatencyMs: 0})
	if !(unknown == 1.0 && fast > slow) {
		t.Fatalf("unknown=%v fast=%v slow=%v", unknown, fast, slow)
	}
}

func TestCompositePrefersFasterWhenLoadEqual(t *testing.T) {
	ch := DefaultChain()
	req := &Request{BaseModel: "m"}
	candidates := []Candidate{
		{WorkerID: "slow", Endpoint: "s", QueueDepth: 0, Healthy: true, Ready: true, EwmaLatencyMs: 300, Models: []string{"m"}},
		{WorkerID: "fast", Endpoint: "f", QueueDepth: 0, Healthy: true, Ready: true, EwmaLatencyMs: 30, Models: []string{"m"}},
	}
	p, err := ch.Pick(nil, req, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if p.WorkerID != "fast" {
		t.Fatalf("got %s", p.WorkerID)
	}
}

func TestHealthFilterMetrics(t *testing.T) {
	// filter without metrics still works
	out := HealthFilter{}.Filter(nil, nil, []Candidate{
		{WorkerID: "a", Healthy: false, Ready: true},
		{WorkerID: "b", Healthy: true, Ready: false},
		{WorkerID: "c", Healthy: true, Ready: true},
	})
	if len(out) != 1 || out[0].WorkerID != "c" {
		t.Fatalf("%+v", out)
	}
}
