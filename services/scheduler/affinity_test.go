package scheduler

import (
	"context"
	"strings"
	"testing"
)

func longPrefix() string {
	// > 1024 bytes of stable system text
	return "system: " + strings.Repeat("You are a clinical triage assistant with shared policy text. ", 40)
}

func TestPrefixKeyStablePrefixIgnoresTail(t *testing.T) {
	prefix := longPrefix()
	if len(prefix) < 1024 {
		t.Fatalf("prefix too short: %d", len(prefix))
	}
	a := prefix + "user: first unique question\n"
	b := prefix + "user: second totally different question\n"
	ka := PrefixKey(a, 1024, 64)
	kb := PrefixKey(b, 1024, 64)
	if ka != kb {
		t.Fatalf("tail change moved key: %d vs %d", ka, kb)
	}
	other := "system: totally different preamble " + strings.Repeat("x", 1024)
	if PrefixKey(other, 1024, 64) == ka {
		t.Fatal("different system should change key")
	}
}

func TestHRWConsistentAndMinimalReshuffle(t *testing.T) {
	workers := []string{"w-a", "w-b", "w-c"}
	key := PrefixKey(longPrefix()+"user: q\n", 1024, 64)
	first := HRWPick(key, workers)
	for i := 0; i < 20; i++ {
		if HRWPick(key, workers) != first {
			t.Fatal("not stable")
		}
	}
	drop := "w-b"
	if first == drop {
		drop = "w-c"
	}
	remaining := make([]string, 0, 2)
	for _, w := range workers {
		if w != drop {
			remaining = append(remaining, w)
		}
	}
	if got := HRWPick(key, remaining); got != first {
		t.Fatalf("removed non-owner %s reshuffled %s -> %s", drop, first, got)
	}
}

func TestAffinityScorerStickyWhenEqualLoad(t *testing.T) {
	ch := DefaultChain()
	req := &Request{BaseModel: "m", Prompt: longPrefix() + "user: ask1\n"}
	candidates := []Candidate{
		{WorkerID: "worker-z", Endpoint: "z", QueueDepth: 0, Healthy: true, Ready: true, Models: []string{"m"}},
		{WorkerID: "worker-a", Endpoint: "a", QueueDepth: 0, Healthy: true, Ready: true, Models: []string{"m"}},
		{WorkerID: "worker-m", Endpoint: "m", QueueDepth: 0, Healthy: true, Ready: true, Models: []string{"m"}},
	}
	p1, err := ch.Pick(context.Background(), req, candidates)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		p, err := ch.Pick(context.Background(), req, candidates)
		if err != nil {
			t.Fatal(err)
		}
		if p.WorkerID != p1.WorkerID {
			t.Fatalf("iter %d: %s vs %s", i, p.WorkerID, p1.WorkerID)
		}
	}
	req2 := &Request{BaseModel: "m", Prompt: longPrefix() + "user: ask2 different\n"}
	if PrefixKey(req.Prompt, 1024, 64) != PrefixKey(req2.Prompt, 1024, 64) {
		t.Fatal("expected same prefix key")
	}
	p2, err := ch.Pick(context.Background(), req2, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if p2.WorkerID != p1.WorkerID {
		t.Fatalf("same prefix key but different worker: %s vs %s", p2.WorkerID, p1.WorkerID)
	}
}
