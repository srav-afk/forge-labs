package health

import (
	"fmt"
	"strings"
	"sync"
)

const (
	keyPrefix = "worker:"
	keySuffix = ":heartbeat"
	keyGlob   = "worker:*:heartbeat"
)

type Heartbeat struct {
	ID         string  `json:"id"`
	BaseModel  string  `json:"base_model"`
	Adapter    *string `json:"adapter"`
	Runtime    string  `json:"runtime"`
	Addr       string  `json:"addr"`
	Ready      bool    `json:"ready"`
	Inflight   int     `json:"inflight"`
	QueueDepth int     `json:"queue_depth"`
	TS         int64   `json:"ts"`
}

func Key(workerID string) string {
	return fmt.Sprintf("%s%s%s", keyPrefix, workerID, keySuffix)
}

func WorkerIDFromKey(key string) (string, bool) {
	if !strings.HasPrefix(key, keyPrefix) || !strings.HasSuffix(key, keySuffix) {
		return "", false
	}
	id := key[len(keyPrefix) : len(key)-len(keySuffix)]
	if id == "" {
		return "", false
	}
	return id, true
}

type Snapshot struct {
	mu      sync.RWMutex
	workers map[string]Heartbeat
}

func NewSnapshot() *Snapshot {
	return &Snapshot{workers: make(map[string]Heartbeat)}
}

func (s *Snapshot) Replace(live map[string]Heartbeat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers = live
}

func (s *Snapshot) Upsert(hb Heartbeat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers[hb.ID] = hb
}

func (s *Snapshot) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workers, id)
}

func (s *Snapshot) Get(id string) (Heartbeat, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hb, ok := s.workers[id]
	return hb, ok
}

func (s *Snapshot) All() []Heartbeat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Heartbeat, 0, len(s.workers))
	for _, hb := range s.workers {
		out = append(out, hb)
	}
	return out
}

func (s *Snapshot) Routable() []Heartbeat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Heartbeat, 0, len(s.workers))
	for _, hb := range s.workers {
		if hb.Ready {
			out = append(out, hb)
		}
	}
	return out
}
