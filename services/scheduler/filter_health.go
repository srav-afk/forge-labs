package scheduler

import "context"

type HealthFilter struct{}

func (HealthFilter) Name() string { return "health" }

func (HealthFilter) Filter(_ context.Context, _ *Request, in []Candidate) []Candidate {
	out := make([]Candidate, 0, len(in))
	for _, c := range in {
		if c.Healthy && c.Ready {
			out = append(out, c)
		}
	}
	return out
}
