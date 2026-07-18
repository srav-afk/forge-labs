package scheduler

import (
	"context"
	"testing"
)

func TestCostScorerPrefersCheaper(t *testing.T) {
	s := NewCostScorer()
	cands := []Candidate{
		{WorkerID: "pod-a", CostPerHour: 1.19, QueueDepth: 3},
		{WorkerID: "pod-b", CostPerHour: 2.69, QueueDepth: 1},
	}
	s.Prepare(context.Background(), nil, cands)
	sa := s.Score(context.Background(), nil, cands[0])
	sb := s.Score(context.Background(), nil, cands[1])
	if sa <= sb {
		t.Fatalf("cheaper should score higher: a=%v b=%v", sa, sb)
	}
	if sb != 0 {
		t.Fatalf("most expensive should be 0, got %v", sb)
	}
}

func TestWorkedExamplePodAWins(t *testing.T) {
	// plan worked example: load weight 0.6, cost 0.4; pod-a running=3 @$1.19, pod-b running=1 @$2.69
	ch := NewConfiguredChain(ChainConfig{
		WeightLoad:     0.6,
		WeightLatency:  0,
		WeightAffinity: 0,
		WeightCost:     0.4,
		AdmissionLimit: 8,
	})
	req := &Request{BaseModel: "llama-3.1-70b"}
	cands := []Candidate{
		{
			WorkerID: "pod-a", Endpoint: "a", Healthy: true, Ready: true,
			QueueDepth: 3, CostPerHour: 1.19, CostClass: "paid",
			Models: []string{"llama-3.1-70b"}, MaxContext: 131072,
		},
		{
			WorkerID: "pod-b", Endpoint: "b", Healthy: true, Ready: true,
			QueueDepth: 1, CostPerHour: 2.69, CostClass: "paid",
			Models: []string{"llama-3.1-70b"}, MaxContext: 131072,
		},
	}
	pick, err := ch.Pick(context.Background(), req, cands)
	if err != nil {
		t.Fatal(err)
	}
	if pick.WorkerID != "pod-a" {
		t.Fatalf("got %s want pod-a (score=%v)", pick.WorkerID, pick.Score)
	}
}

func TestFreeMacWinsWhenCapable(t *testing.T) {
	ch := NewConfiguredChain(ChainConfig{
		WeightLoad: 0.6, WeightCost: 0.4, AdmissionLimit: 8,
	})
	req := &Request{BaseModel: "qwen2.5-7b"}
	cands := []Candidate{
		{
			WorkerID: "mac-1", Endpoint: "m", Healthy: true, Ready: true,
			QueueDepth: 2, CostPerHour: 0, CostClass: "free",
			Models: []string{"qwen2.5-7b"}, MaxContext: 32768,
		},
		{
			WorkerID: "pod-a", Endpoint: "a", Healthy: true, Ready: true,
			QueueDepth: 0, CostPerHour: 1.19, CostClass: "paid",
			Models: []string{"qwen2.5-7b", "llama-3.1-70b"}, MaxContext: 131072,
		},
	}
	pick, err := ch.Pick(context.Background(), req, cands)
	if err != nil {
		t.Fatal(err)
	}
	if pick.WorkerID != "mac-1" {
		t.Fatalf("got %s want mac-1", pick.WorkerID)
	}
}

func TestCapabilityFilterDropsIncapable(t *testing.T) {
	f := CapabilityFilter{}
	req := &Request{BaseModel: "llama-3.1-70b", MinContext: 100000}
	in := []Candidate{
		{WorkerID: "mac-1", Models: []string{"qwen2.5-7b"}, MaxContext: 32768},
		{WorkerID: "pod-a", Models: []string{"llama-3.1-70b"}, MaxContext: 131072},
		{WorkerID: "pod-short", Models: []string{"llama-3.1-70b"}, MaxContext: 8192},
	}
	out := f.Filter(context.Background(), req, in)
	if len(out) != 1 || out[0].WorkerID != "pod-a" {
		t.Fatalf("%+v", out)
	}
}

func TestSeventyBSkipsMac(t *testing.T) {
	ch := DefaultChain()
	req := &Request{BaseModel: "meta-llama/Llama-3.1-70B-Instruct"}
	cands := []Candidate{
		{
			WorkerID: "mac-1", Endpoint: "m", Healthy: true, Ready: true,
			CostPerHour: 0, Models: []string{"qwen2.5-7b", "llama-3.2-3b"},
		},
		{
			WorkerID: "pod-a", Endpoint: "100.81.4.12:50051", Healthy: true, Ready: true,
			CostPerHour: 1.19, Models: []string{"meta-llama/Llama-3.1-70B-Instruct"},
		},
	}
	pick, err := ch.Pick(context.Background(), req, cands)
	if err != nil {
		t.Fatal(err)
	}
	if pick.WorkerID != "pod-a" {
		t.Fatalf("got %s", pick.WorkerID)
	}
}
