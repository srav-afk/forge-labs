package scheduler

import (
	"context"
	"sync"
)

type CostScorer struct {
	mu     sync.Mutex
	maxUSD float64
}

func NewCostScorer() *CostScorer {
	return &CostScorer{}
}

func (s *CostScorer) Name() string { return "cost" }

func (s *CostScorer) Prepare(_ context.Context, _ *Request, candidates []Candidate) {
	max := 0.0
	for _, c := range candidates {
		if c.CostPerHour > max {
			max = c.CostPerHour
		}
	}
	s.mu.Lock()
	s.maxUSD = max
	s.mu.Unlock()
}

func (s *CostScorer) Score(_ context.Context, _ *Request, c Candidate) float64 {
	s.mu.Lock()
	max := s.maxUSD
	s.mu.Unlock()
	if max <= 0 {
		return 1.0
	}
	ratio := c.CostPerHour / max
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	return 1.0 - ratio
}
