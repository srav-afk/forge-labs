package scheduler

import (
	"context"
	"errors"
	"testing"
)

func TestAdmissionFilterRemovesAtCap(t *testing.T) {
	f := AdmissionFilter{Limit: 2}
	out := f.Filter(context.Background(), nil, []Candidate{
		{WorkerID: "a", QueueDepth: 1, Healthy: true, Ready: true, Models: []string{"m"}},
		{WorkerID: "b", QueueDepth: 2, Healthy: true, Ready: true, Models: []string{"m"}},
		{WorkerID: "c", QueueDepth: 0, Healthy: true, Ready: true, Models: []string{"m"}},
	})
	if len(out) != 2 {
		t.Fatalf("len=%d", len(out))
	}
	for _, c := range out {
		if c.WorkerID == "b" {
			t.Fatal("b should be filtered")
		}
	}
}

func TestPickAdmissionRejectedWhenAllAtCap(t *testing.T) {
	ch := NewConfiguredChain(ChainConfig{
		WeightLoad:     1,
		WeightLatency:  0,
		AdmissionLimit: 2,
	})
	_, err := ch.Pick(context.Background(), &Request{BaseModel: "m"}, []Candidate{
		{WorkerID: "a", QueueDepth: 2, Healthy: true, Ready: true, Models: []string{"m"}},
		{WorkerID: "b", QueueDepth: 5, Healthy: true, Ready: true, Models: []string{"m"}},
	})
	if !errors.Is(err, ErrAdmissionRejected) {
		t.Fatalf("err=%v", err)
	}
}

func TestPickStillNoCapacityWhenNoModel(t *testing.T) {
	ch := NewConfiguredChain(ChainConfig{
		WeightLoad:     1,
		AdmissionLimit: 8,
	})
	_, err := ch.Pick(context.Background(), &Request{BaseModel: "missing"}, []Candidate{
		{WorkerID: "a", QueueDepth: 0, Healthy: true, Ready: true, Models: []string{"other"}},
	})
	if !errors.Is(err, ErrNoCapacity) {
		t.Fatalf("err=%v want no capacity", err)
	}
}
