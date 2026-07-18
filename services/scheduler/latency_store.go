package scheduler

import (
	"sync"
	"time"
)

type LatencyStore struct {
	tau     time.Duration
	seed    float64
	metrics *Metrics
	m       sync.Map
}

func NewLatencyStore(tau time.Duration, metrics *Metrics) *LatencyStore {
	if tau <= 0 {
		tau = 10 * time.Second
	}
	return &LatencyStore{tau: tau, seed: 0, metrics: metrics}
}

func (s *LatencyStore) Observe(workerID string, sampleMs float64) {
	if workerID == "" || sampleMs < 0 {
		return
	}
	e := s.getOrCreate(workerID)
	e.Update(sampleMs, time.Now())
	if s.metrics != nil {
		s.metrics.SetEWMA(workerID, e.Value())
	}
}

func (s *LatencyStore) Get(workerID string) float64 {
	if v, ok := s.m.Load(workerID); ok {
		return v.(*EWMA).Value()
	}
	return 0
}

func (s *LatencyStore) getOrCreate(workerID string) *EWMA {
	if v, ok := s.m.Load(workerID); ok {
		return v.(*EWMA)
	}
	e := NewEWMA(s.tau, s.seed)
	actual, _ := s.m.LoadOrStore(workerID, e)
	return actual.(*EWMA)
}
