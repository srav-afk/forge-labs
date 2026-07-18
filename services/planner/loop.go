package planner

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync/atomic"
	"time"

	"github.com/srav-afk/forge-labs/services/routing"
)

type Service struct {
	objectives *ObjectiveStore
	policies   *PolicyStore
	holder     *routing.SnapshotHolder
	policyOut  *routing.PolicyHolder
	version    atomic.Uint64
	interval   time.Duration
}

func NewService(
	objectives *ObjectiveStore,
	policies *PolicyStore,
	holder *routing.SnapshotHolder,
	policyOut *routing.PolicyHolder,
) *Service {
	return &Service{
		objectives: objectives,
		policies:   policies,
		holder:     holder,
		policyOut:  policyOut,
		interval:   45 * time.Second,
	}
}

func (s *Service) Start(ctx context.Context) {
	if live, err := s.policies.LoadLive(ctx); err == nil && live != nil {
		s.version.Store(live.Version)
		if s.policyOut != nil {
			s.policyOut.StoreIfNewer(toRoutingPolicy(live))
		}
	}
	go s.loop(ctx)
}

func (s *Service) loop(ctx context.Context) {
	s.tick(ctx)
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

func (s *Service) TickNow(ctx context.Context) error {
	return s.tick(ctx)
}

func (s *Service) tick(ctx context.Context) error {
	if err := s.objectives.Reload(ctx); err != nil {
		log.Printf("planner: objective reload: %v", err)
	}
	obj := s.objectives.Get()
	if obj.EvalIntervalSec > 0 {
		s.interval = time.Duration(obj.EvalIntervalSec) * time.Second
	}

	snap := s.holder.Load()
	policy, score, reason := s.plan(obj, snap)
	ver := s.version.Add(1)
	policy.Version = ver
	policy.GeneratedAt = time.Now().UTC()
	policy.ObjectiveHash = HashObjective(obj)
	policy.Reason = reason

	if err := s.policies.Publish(ctx, policy, score); err != nil {
		log.Printf("planner: publish: %v", err)
		return err
	}
	if s.policyOut != nil {
		s.policyOut.StoreIfNewer(toRoutingPolicy(&policy))
	}
	log.Printf("planner: published policy v%d score=%.3f reason=%s", ver, score, reason)
	return nil
}

func (s *Service) plan(obj Objective, snap *routing.RoutingSnapshot) (RoutingPolicy, float64, string) {
	p := RoutingPolicy{
		WeightLoad:     clamp01(obj.WeightLoad),
		WeightLatency:  clamp01(obj.WeightLatency),
		WeightCost:     clamp01(obj.WeightCost),
		WeightAffinity: clamp01(obj.WeightAffinity),
		Models:         map[string]ModelPolicy{},
	}
	if p.WeightLoad+p.WeightLatency+p.WeightCost+p.WeightAffinity == 0 {
		p.WeightLoad, p.WeightLatency, p.WeightCost, p.WeightAffinity = 0.5, 0.2, 0.2, 0.1
	}

	if snap == nil || len(snap.Workers) == 0 {
		return p, 0, "no workers; emit objective weights only"
	}

	byModel := map[string][]routing.WorkerView{}
	for _, w := range snap.Workers {
		if !w.Healthy || !w.Ready || w.BaseModel == "" {
			continue
		}
		if obj.MaxCostPerHour > 0 && w.CostPerHour > obj.MaxCostPerHour {
			continue
		}
		byModel[w.BaseModel] = append(byModel[w.BaseModel], w)
	}

	totalScore := 0.0
	for model, workers := range byModel {
		weights := scoreWorkers(obj, workers)
		p.Models[model] = ModelPolicy{
			Weights:           weights,
			Affinity:          "prefix_hash",
			ConcurrencyTarget: 4,
			MaxQueueDepth:     16,
		}
		for _, sc := range weights {
			totalScore += sc
		}
	}
	reason := fmt.Sprintf("objective latency=%.2f cost=%.2f load=%.2f models=%d",
		obj.WeightLatency, obj.WeightCost, obj.WeightLoad, len(p.Models))
	return p, totalScore, reason
}

func scoreWorkers(obj Objective, workers []routing.WorkerView) map[string]float64 {
	maxCost := 0.0
	maxLoad := 1.0
	for _, w := range workers {
		if w.CostPerHour > maxCost {
			maxCost = w.CostPerHour
		}
		load := float64(w.QueueDepth + w.InFlight)
		if load > maxLoad {
			maxLoad = load
		}
	}
	raw := map[string]float64{}
	sum := 0.0
	for _, w := range workers {
		load := float64(w.QueueDepth + w.InFlight)
		loadScore := 1.0 / (1.0 + load)
		costScore := 1.0
		if maxCost > 0 {
			costScore = 1.0 - (w.CostPerHour / maxCost)
		}
		latScore := 1.0
		sc := obj.WeightLoad*loadScore + obj.WeightCost*costScore + obj.WeightLatency*latScore
		if sc < 0.01 {
			sc = 0.01
		}
		raw[w.ID] = sc
		sum += sc
	}
	out := map[string]float64{}
	if sum <= 0 {
		eq := 1.0 / float64(len(workers))
		for _, w := range workers {
			out[w.ID] = eq
		}
		return out
	}
	for id, sc := range raw {
		out[id] = sc / sum
	}
	return out
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

func toRoutingPolicy(p *RoutingPolicy) *routing.RoutingPolicy {
	if p == nil {
		return nil
	}
	out := &routing.RoutingPolicy{
		Version:        p.Version,
		WeightLoad:     p.WeightLoad,
		WeightLatency:  p.WeightLatency,
		WeightCost:     p.WeightCost,
		WeightAffinity: p.WeightAffinity,
		WorkerWeights:  map[string]map[string]float64{},
	}
	for model, mp := range p.Models {
		out.WorkerWeights[model] = mp.Weights
	}
	return out
}
