package scheduler

import (
	"context"
	"strings"
)

type ModelFilter struct{}

func (ModelFilter) Name() string { return "model" }

func (ModelFilter) Filter(_ context.Context, req *Request, in []Candidate) []Candidate {
	if req == nil || req.BaseModel == "" {
		return nil
	}
	want := req.BaseModel
	if req.Adapter != "" {
		want = req.BaseModel + "#" + req.Adapter
	}
	out := make([]Candidate, 0, len(in))
	for _, c := range in {
		if servesModel(c.Models, want, req.BaseModel, req.Adapter) {
			out = append(out, c)
		}
	}
	return out
}

func servesModel(models []string, want, base, adapter string) bool {
	for _, m := range models {
		if m == want || m == base {
			return true
		}
		if adapter == "" && strings.HasPrefix(m, base+"#") {
			return true
		}
		mb, ma, ok := splitModel(m)
		if !ok {
			continue
		}
		if mb == base && (adapter == "" || ma == adapter) {
			return true
		}
	}
	return false
}

func splitModel(id string) (base, adapter string, ok bool) {
	if id == "" {
		return "", "", false
	}
	if i := strings.Index(id, "#"); i >= 0 {
		return id[:i], id[i+1:], true
	}
	return id, "", true
}
