package scheduler

import "github.com/srav-afk/forge-labs/services/routing"

type LoadSource interface {
	Get(workerID string) int
}

type LatencySource interface {
	Get(workerID string) float64
}

func CandidatesFromSnapshot(snap *routing.RoutingSnapshot, load LoadSource, latency LatencySource) []Candidate {
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
			ewma := 0.0
			if latency != nil {
				ewma = latency.Get(w.ID)
			}
			c = &Candidate{
				WorkerID:      w.ID,
				Endpoint:      w.Endpoint,
				QueueDepth:    w.QueueDepth + w.InFlight + extra,
				Healthy:       w.Healthy,
				Ready:         w.Ready,
				EwmaLatencyMs: ewma,
				Models:        nil,
				MaxContext:    w.MaxContext,
				CostPerHour:   w.CostPerHour,
				CostClass:     w.CostClass,
				Runtime:       w.Runtime,
				VRAMGB:        w.VRAMGB,
				GPU:           w.GPU,
				Capabilities:  w.Capabilities,
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
			if w.MaxContext > c.MaxContext {
				c.MaxContext = w.MaxContext
			}
			if w.CostPerHour > 0 {
				c.CostPerHour = w.CostPerHour
			}
			if w.CostClass != "" {
				c.CostClass = w.CostClass
			}
			if w.Runtime != "" {
				c.Runtime = w.Runtime
			}
			if w.VRAMGB > c.VRAMGB {
				c.VRAMGB = w.VRAMGB
			}
			if w.GPU != "" {
				c.GPU = w.GPU
			}
			if len(w.Capabilities) > 0 {
				c.Capabilities = w.Capabilities
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
