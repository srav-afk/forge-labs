package scheduler

import (
	"context"
	"errors"
	"sort"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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
	WeightCost     float64
	WeightPolicy   float64
	LatencyRefMs   float64
	AdmissionLimit int
	AffinityWindow int
	AffinityBlock  int
	Metrics        *Metrics
	Policy         *PolicyScorer
}

func DefaultChain() *Chain {
	return NewConfiguredChain(ChainConfig{
		WeightLoad:     0.45,
		WeightLatency:  0.15,
		WeightAffinity: 0.1,
		WeightCost:     0.15,
		WeightPolicy:   0.15,
		LatencyRefMs:   100,
		AdmissionLimit: 4,
		AffinityWindow: 1024,
		AffinityBlock:  64,
	})
}

func NewConfiguredChain(cfg ChainConfig) *Chain {
	wl, wlat, waff, wc, wp := cfg.WeightLoad, cfg.WeightLatency, cfg.WeightAffinity, cfg.WeightCost, cfg.WeightPolicy
	if wl <= 0 && wlat <= 0 && waff <= 0 && wc <= 0 && wp <= 0 {
		wl, wlat, waff, wc, wp = 0.45, 0.15, 0.1, 0.15, 0.15
	}
	filters := []Filter{
		HealthFilter{Metrics: cfg.Metrics},
		ModelFilter{},
		CapabilityFilter{Metrics: cfg.Metrics},
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
	if wc > 0 {
		scorers = append(scorers, WeightedScorer{
			Scorer: NewCostScorer(),
			Weight: wc,
		})
	}
	if wp > 0 && cfg.Policy != nil {
		scorers = append(scorers, WeightedScorer{Scorer: cfg.Policy, Weight: wp})
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
	if ctx == nil {
		ctx = context.Background()
	}
	tr := otel.Tracer("forge-controlplane")
	ctx, root := tr.Start(ctx, "scheduler.rank")
	defer root.End()
	if req != nil {
		root.SetAttributes(
			attribute.String("base_model", req.BaseModel),
			attribute.String("adapter", req.Adapter),
			attribute.Int("candidates_in", len(candidates)),
		)
	}

	surviving := candidates
	ctx, filterSpan := tr.Start(ctx, "scheduler.filter")
	for _, f := range ch.Filters {
		before := len(surviving)
		surviving = f.Filter(ctx, req, surviving)
		filterSpan.AddEvent("filter", trace.WithAttributes(
			attribute.String("name", f.Name()),
			attribute.Int("before", before),
			attribute.Int("after", len(surviving)),
		))
		if len(surviving) == 0 {
			filterSpan.SetStatus(codes.Error, f.Name())
			filterSpan.End()
			if f.Name() == "admission" {
				root.SetStatus(codes.Error, "admission rejected")
				return nil, ErrAdmissionRejected
			}
			root.SetStatus(codes.Error, "no capacity")
			return nil, ErrNoCapacity
		}
	}
	filterSpan.SetAttributes(attribute.Int("candidates_out", len(surviving)))
	filterSpan.End()

	ctx, scoreSpan := tr.Start(ctx, "scheduler.score")
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
	if len(out) > 0 {
		attrs := []attribute.KeyValue{
			attribute.String("chosen_worker", out[0].WorkerID),
			attribute.Float64("score", out[0].Score),
		}
		if req != nil && req.PreferredWorker != "" {
			attrs = append(attrs, attribute.String("preferred_worker", req.PreferredWorker))
		}
		scoreSpan.SetAttributes(attrs...)
		root.SetAttributes(attribute.String("chosen_worker", out[0].WorkerID))
	}
	scoreSpan.End()
	return out, nil
}
