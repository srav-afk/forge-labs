package health

import (
	"testing"
	"time"
)

func TestKeyRoundTrip(t *testing.T) {
	key := Key("mac-studio-01")
	if key != "worker:mac-studio-01:heartbeat" {
		t.Fatalf("key = %q", key)
	}
	id, ok := WorkerIDFromKey(key)
	if !ok || id != "mac-studio-01" {
		t.Fatalf("WorkerIDFromKey = %q, %v", id, ok)
	}
	if _, ok := WorkerIDFromKey("not-a-heartbeat"); ok {
		t.Fatal("expected false for non-heartbeat key")
	}
}

func TestSnapshotRoutable(t *testing.T) {
	s := NewSnapshot()
	s.Replace(map[string]Heartbeat{
		"a": {ID: "a", BaseModel: "qwen3:8b", Runtime: "ollama", Ready: true, TS: time.Now().UnixMilli()},
		"b": {ID: "b", BaseModel: "qwen3:8b", Runtime: "ollama", Ready: false, TS: time.Now().UnixMilli()},
	})
	all := s.All()
	if len(all) != 2 {
		t.Fatalf("All len = %d", len(all))
	}
	routable := s.Routable()
	if len(routable) != 1 || routable[0].ID != "a" {
		t.Fatalf("Routable = %+v", routable)
	}
	s.Remove("a")
	if len(s.All()) != 1 {
		t.Fatalf("after Remove All len = %d", len(s.All()))
	}
}
