package scheduler

import (
	"context"
	"errors"
)

var ErrAdmissionRejected = errors.New("admission rejected")

type AdmissionFilter struct {
	Limit   int
	Metrics *Metrics
}

func (AdmissionFilter) Name() string { return "admission" }

func (f AdmissionFilter) Filter(_ context.Context, _ *Request, in []Candidate) []Candidate {
	limit := f.Limit
	if limit <= 0 {
		return in
	}
	out := make([]Candidate, 0, len(in))
	for _, c := range in {
		if c.QueueDepth < limit {
			out = append(out, c)
			continue
		}
		if f.Metrics != nil {
			f.Metrics.IncFiltered("at_capacity")
		}
	}
	return out
}
