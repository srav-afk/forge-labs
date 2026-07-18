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

type ChainConfig struct {
	WeightLoad     float64
	WeightLatency  float64
	WeightAffinity float64
	LatencyRefMs   float64
	AdmissionLimit int
	AffinityWindow int
	AffinityBlock  int
	Metrics        *Metrics
}

func DefaultChain() *Chain {
	return NewConfiguredChain(ChainConfig{
		WeightLoad:     0.6,
		WeightLatency:  0.2,
		WeightAffinity: 0.2,
		LatencyRefMs:   100,
		AdmissionLimit: 4,
		AffinityWindow: 1024,
		AffinityBlock:  64,
	})
}

func NewConfiguredChain(cfg ChainConfig) *Chain {
	wl, wlat, waff := cfg.WeightLoad, cfg.WeightLatency, cfg.WeightAffinity
	if wl <= 0 && wlat <= 0 && waff <= 0 {
		wl, wlat, waff = 0.6, 0.2, 0.2
	}
	filters := []Filter{
		HealthFilter{Metrics: cfg.Metrics},
		ModelFilter{},
	}
	if cfg.AdmissionLimit > 0 {
		filters = append(filters, AdmissionFilter{Limit: cfg.AdmissionLimit, Metrics: cfg.Metrics})
	}
	scorers := []WeightedScorer{
		{Scorer: LeastLoaded{}, Weight: wl},
		{Scorer: NewLatencyScorer(cfg.LatencyRefMs), Weight: wlat},
	}
	if waff > 0 {
		scorers = append(scorers, WeightedScorer{
			Scorer: NewAffinityScorer(cfg.AffinityWindow, cfg.AffinityBlock),
			Weight: waff,
		})
	}
	return NewChain(filters, scorers)
}

type preparer interface {
	Prepare(ctx context.Context, req *Request, candidates []Candidate)
}

type PickResult struct {
	WorkerID string
	Endpoint string
	Score    float64
}

func (ch *Chain) Pick(ctx context.Context, req *Request, candidates []Candidate) (PickResult, error) {
	ranked, err := ch.Rank(ctx, req, candidates)
	if err != nil {
		return PickResult{}, err
	}
	return ranked[0], nil
}

func (ch *Chain) PickWithMetrics(ctx context.Context, req *Request, candidates []Candidate, metrics *Metrics) (PickResult, error) {
	ranked, err := ch.Rank(ctx, req, candidates)
	if err != nil {
		return PickResult{}, err
	}
	p := ranked[0]
	if metrics != nil {
		metrics.SetScore(p.WorkerID, p.Score)
		metrics.IncRoutingDecision()
		if req != nil && req.PreferredWorker != "" && p.WorkerID == req.PreferredWorker {
			metrics.IncAffinityHit()
		}
	}
	return p, nil
}

func (ch *Chain) Rank(ctx context.Context, req *Request, candidates []Candidate) ([]PickResult, error) {
	surviving := candidates
	for _, f := range ch.Filters {
		surviving = f.Filter(ctx, req, surviving)
		if len(surviving) == 0 {
			if f.Name() == "admission" {
				return nil, ErrAdmissionRejected
			}
			return nil, ErrNoCapacity
		}
	}

	for _, ws := range ch.Scorers {
		if p, ok := ws.Scorer.(preparer); ok {
			p.Prepare(ctx, req, surviving)
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

	out := make([]PickResult, 0, len(results))
	for _, r := range results {
		out = append(out, PickResult{
			WorkerID: r.c.WorkerID,
			Endpoint: r.c.Endpoint,
			Score:    r.total,
		})
	}
	return out, nil
}
