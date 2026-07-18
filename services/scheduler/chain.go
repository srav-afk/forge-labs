package scheduler

import (
	"context"
	"errors"
	"sort"
)

var ErrNoCapacity = errors.New("no capacity")

type Chain struct {
	Filters []Filter
	Scorers []WeightedScorer
}

func NewChain(filters []Filter, scorers []WeightedScorer) *Chain {
	return &Chain{Filters: filters, Scorers: scorers}
}

func DefaultChain() *Chain {
	return NewChain(
		[]Filter{HealthFilter{}, ModelFilter{}},
		[]WeightedScorer{{Scorer: LeastLoaded{}, Weight: 1}},
	)
}

type PickResult struct {
	WorkerID string
	Endpoint string
	Score    float64
}

func (ch *Chain) Pick(ctx context.Context, req *Request, candidates []Candidate) (PickResult, error) {
	surviving := candidates
	for _, f := range ch.Filters {
		surviving = f.Filter(ctx, req, surviving)
		if len(surviving) == 0 {
			return PickResult{}, ErrNoCapacity
		}
	}

	type scored struct {
		c     Candidate
		total float64
	}
	results := make([]scored, 0, len(surviving))
	for _, c := range surviving {
		var total float64
		for _, ws := range ch.Scorers {
			w := ws.Weight
			if w == 0 {
				w = 1
			}
			total += w * ws.Scorer.Score(ctx, req, c)
		}
		results = append(results, scored{c: c, total: total})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].total != results[j].total {
			return results[i].total > results[j].total
		}
		return results[i].c.WorkerID < results[j].c.WorkerID
	})

	best := results[0]
	return PickResult{
		WorkerID: best.c.WorkerID,
		Endpoint: best.c.Endpoint,
		Score:    best.total,
	}, nil
}
