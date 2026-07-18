package scheduler

import (
	"context"

	"github.com/srav-afk/forge-labs/services/routing"
)

type PolicyScorer struct {
	holder *routing.PolicyHolder
}

func NewPolicyScorer(holder *routing.PolicyHolder) *PolicyScorer {
	return &PolicyScorer{holder: holder}
}

func (s *PolicyScorer) Name() string { return "policy-weight" }

func (s *PolicyScorer) Score(_ context.Context, req *Request, c Candidate) float64 {
	if s == nil || s.holder == nil || req == nil {
		return 1.0
	}
	p := s.holder.Load()
	if p == nil || len(p.WorkerWeights) == 0 {
		return 1.0
	}
	byWorker, ok := p.WorkerWeights[req.BaseModel]
	if !ok {
		return 1.0
	}
	if w, ok := byWorker[c.WorkerID]; ok {
		return w
	}
	return 0.01
}
