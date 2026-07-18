package scheduler

import "context"

type HealthFilter struct {
	Metrics *Metrics
}

func (HealthFilter) Name() string { return "health" }

func (f HealthFilter) Filter(_ context.Context, _ *Request, in []Candidate) []Candidate {
	out := make([]Candidate, 0, len(in))
	for _, c := range in {
		if !c.Healthy {
			if f.Metrics != nil {
				f.Metrics.IncFiltered("unhealthy")
			}
			continue
		}
		if !c.Ready {
			if f.Metrics != nil {
				f.Metrics.IncFiltered("draining")
			}
			continue
		}
		out = append(out, c)
	}
	return out
}
