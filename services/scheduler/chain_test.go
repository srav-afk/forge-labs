package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/srav-afk/forge-labs/services/routing"
)

func TestPickLeastLoadedDeterministic(t *testing.T) {
	ch := DefaultChain()
	req := &Request{BaseModel: "llama3.1:8b"}
	candidates := []Candidate{
		{WorkerID: "worker-b", Endpoint: "b:1", QueueDepth: 0, Healthy: true, Ready: true, Models: []string{"llama3.1:8b"}},
		{WorkerID: "worker-a", Endpoint: "a:1", QueueDepth: 0, Healthy: true, Ready: true, Models: []string{"llama3.1:8b"}},
	}
	// equal scores → lexicographically smaller id
	p, err := ch.Pick(context.Background(), req, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if p.WorkerID != "worker-a" {
		t.Fatalf("got %s want worker-a", p.WorkerID)
	}

	// same input always same output
	for i := 0; i < 20; i++ {
		p2, err := ch.Pick(context.Background(), req, candidates)
		if err != nil || p2.WorkerID != "worker-a" {
			t.Fatalf("iter %d: %+v %v", i, p2, err)
		}
	}
}

func TestPickPrefersLowerQueue(t *testing.T) {
	ch := DefaultChain()
	req := &Request{BaseModel: "m"}
	candidates := []Candidate{
		{WorkerID: "busy", Endpoint: "b", QueueDepth: 3, Healthy: true, Ready: true, Models: []string{"m"}},
		{WorkerID: "idle", Endpoint: "i", QueueDepth: 0, Healthy: true, Ready: true, Models: []string{"m"}},
	}
	p, err := ch.Pick(context.Background(), req, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if p.WorkerID != "idle" {
		t.Fatalf("got %s", p.WorkerID)
	}
	// busy score 0.25, idle 1.0
	if p.Score != 1.0 {
		t.Fatalf("score=%v", p.Score)
	}
}

func TestPickNoCapacityUnhealthyOrWrongModel(t *testing.T) {
	ch := DefaultChain()
	req := &Request{BaseModel: "wanted"}
	_, err := ch.Pick(context.Background(), req, []Candidate{
		{WorkerID: "w1", Healthy: false, Ready: true, Models: []string{"wanted"}},
		{WorkerID: "w2", Healthy: true, Ready: true, Models: []string{"other"}},
		{WorkerID: "w3", Healthy: true, Ready: false, Models: []string{"wanted"}},
	})
	if !errors.Is(err, ErrNoCapacity) {
		t.Fatalf("err=%v", err)
	}
}

func TestCandidatesMergeInflight(t *testing.T) {
	inf := routing.NewInflightTracker()
	inf.Inc("w1")
	inf.Inc("w1")
	snap := &routing.RoutingSnapshot{
		Workers: []routing.WorkerView{
			{ID: "w1", Endpoint: "e1", BaseModel: "m", Healthy: true, Ready: true, QueueDepth: 1, InFlight: 0},
			{ID: "w1", Endpoint: "e1", BaseModel: "m2", Healthy: true, Ready: true},
			{ID: "w2", Endpoint: "e2", BaseModel: "m", Healthy: true, Ready: true},
		},
	}
	cs := CandidatesFromSnapshot(snap, inf, nil)
	if len(cs) != 2 {
		t.Fatalf("len=%d", len(cs))
	}
	var w1 Candidate
	for _, c := range cs {
		if c.WorkerID == "w1" {
			w1 = c
		}
	}
	// 1 (queue) + 0 (inflight snap) + 2 (local) = 3
	if w1.QueueDepth != 3 {
		t.Fatalf("depth=%d models=%v", w1.QueueDepth, w1.Models)
	}
	if len(w1.Models) != 2 {
		t.Fatalf("models=%v", w1.Models)
	}
}

func TestLeastLoadedScore(t *testing.T) {
	s := LeastLoaded{}
	if s.Score(context.Background(), nil, Candidate{QueueDepth: 0}) != 1.0 {
		t.Fatal("0")
	}
	if s.Score(context.Background(), nil, Candidate{QueueDepth: 3}) != 0.25 {
		t.Fatal("3")
	}
}
