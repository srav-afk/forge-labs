package scheduler

import "context"

type LatencyScorer struct {
	refMs float64
}

func NewLatencyScorer(refMs float64) LatencyScorer {
	if refMs <= 0 {
		refMs = 100
	}
	return LatencyScorer{refMs: refMs}
}

func (s LatencyScorer) Name() string { return "latency" }

func (s LatencyScorer) Score(_ context.Context, _ *Request, c Candidate) float64 {
	ewma := c.EwmaLatencyMs
	if ewma <= 0 {
		return 1.0
	}
	return 1.0 / (1.0 + ewma/s.refMs)
}
