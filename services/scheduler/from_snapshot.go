package scheduler

import "github.com/srav-afk/forge-labs/services/routing"

type LoadSource interface {
	Get(workerID string) int
}

func CandidatesFromSnapshot(snap *routing.RoutingSnapshot, load LoadSource) []Candidate {
	if snap == nil {
		return nil
	}
	byID := map[string]*Candidate{}
	order := make([]string, 0)
	for _, w := range snap.Workers {
		c, ok := byID[w.ID]
		if !ok {
			extra := 0
			if load != nil {
				extra = load.Get(w.ID)
			}
			c = &Candidate{
				WorkerID:   w.ID,
				Endpoint:   w.Endpoint,
				QueueDepth: w.QueueDepth + w.InFlight + extra,
				Healthy:    w.Healthy,
				Ready:      w.Ready,
				Models:     nil,
			}
			byID[w.ID] = c
			order = append(order, w.ID)
		} else {
			if w.Endpoint != "" {
				c.Endpoint = w.Endpoint
			}
			c.Healthy = c.Healthy || w.Healthy
			c.Ready = c.Ready || w.Ready
			depth := w.QueueDepth + w.InFlight
			if load != nil {
				depth += load.Get(w.ID)
			}
			if depth > c.QueueDepth {
				c.QueueDepth = depth
			}
		}
		if w.BaseModel == "" {
			continue
		}
		id := w.BaseModel
		if w.Adapter != "" {
			id = w.BaseModel + "#" + w.Adapter
		}
		if !contains(c.Models, id) {
			c.Models = append(c.Models, id)
		}
	}
	out := make([]Candidate, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
