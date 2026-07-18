package scheduler

import (
	"context"
	"strings"
)

type CapabilityFilter struct {
	Metrics *Metrics
}

func (CapabilityFilter) Name() string { return "capability" }

func (f CapabilityFilter) Filter(_ context.Context, req *Request, in []Candidate) []Candidate {
	if req == nil {
		return in
	}
	out := make([]Candidate, 0, len(in))
	for _, c := range in {
		if !satisfiesCapability(req, c) {
			if f.Metrics != nil {
				f.Metrics.IncFiltered("capability")
			}
			continue
		}
		out = append(out, c)
	}
	return out
}

func satisfiesCapability(req *Request, c Candidate) bool {
	if req.MinContext > 0 && c.MaxContext > 0 && c.MaxContext < req.MinContext {
		return false
	}
	if req.BaseModel == "" {
		return true
	}
	if len(c.Models) == 0 {
		return false
	}
	want := req.BaseModel
	if req.Adapter != "" {
		want = req.BaseModel + "#" + req.Adapter
	}
	return servesModel(c.Models, want, req.BaseModel, req.Adapter)
}

func CapRuntime(c Candidate) string {
	if c.Runtime != "" {
		return strings.ToLower(c.Runtime)
	}
	if c.Capabilities != nil {
		return strings.ToLower(c.Capabilities["runtime"])
	}
	return ""
}
