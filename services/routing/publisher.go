package routing

import (
	"context"
	"encoding/json"
	"log"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/srav-afk/forge-labs/services/health"
	"github.com/srav-afk/forge-labs/services/registry"
)

type Publisher struct {
	rdb      *redis.Client
	repo     registry.WorkerRepository
	health   *health.Service
	holder   *SnapshotHolder
	metrics  *Metrics
	interval time.Duration
	epoch    atomic.Uint64
}

func NewPublisher(
	rdb *redis.Client,
	repo registry.WorkerRepository,
	healthSvc *health.Service,
	holder *SnapshotHolder,
	metrics *Metrics,
	interval time.Duration,
) *Publisher {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	return &Publisher{
		rdb:      rdb,
		repo:     repo,
		health:   healthSvc,
		holder:   holder,
		metrics:  metrics,
		interval: interval,
	}
}

func (p *Publisher) Start(ctx context.Context) {
	if err := p.tick(ctx); err != nil {
		log.Printf("routing: initial publish: %v", err)
	}
	t := time.NewTicker(p.interval)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := p.tick(ctx); err != nil {
					log.Printf("routing: publish: %v", err)
				}
			}
		}
	}()
}

func (p *Publisher) tick(ctx context.Context) error {
	snap, err := p.buildSnapshot(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	if err := p.rdb.Publish(ctx, Channel, payload).Err(); err != nil {
		return err
	}
	p.holder.StoreIfNewer(snap)
	if p.metrics != nil {
		p.metrics.IncPublish()
	}
	return nil
}

func (p *Publisher) buildSnapshot(ctx context.Context) (*RoutingSnapshot, error) {
	workers, err := p.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	hbByID := map[string]health.Heartbeat{}
	if p.health != nil {
		for _, hb := range p.health.Snapshot().All() {
			hbByID[hb.ID] = hb
		}
	}

	views := make([]WorkerView, 0, len(workers))
	for _, w := range workers {
		hb, live := hbByID[w.ID]
		queue, inflight := 0, 0
		ready := false
		if live {
			queue = hb.QueueDepth
			inflight = hb.Inflight
			ready = hb.Ready
		}
		endpoint := w.Endpoint
		if live && hb.Addr != "" {
			endpoint = hb.Addr
		}
		caps := ParseCapabilities([]byte(w.Capabilities), w.RuntimeKind)

		if len(w.Models) == 0 {
			views = append(views, WorkerView{
				ID:           w.ID,
				Endpoint:     endpoint,
				Healthy:      live,
				Ready:        ready,
				QueueDepth:   queue,
				InFlight:     inflight,
				MaxContext:   caps.MaxContext,
				CostPerHour:  caps.Cost.PerHourUSD,
				CostClass:    caps.Cost.Class,
				Runtime:      caps.Runtime,
				VRAMGB:       caps.VRAMGB,
				GPU:          caps.GPU,
				Capabilities: caps.Raw,
			})
			continue
		}
		for _, m := range w.Models {
			maxCtx := m.MaxContext
			if maxCtx == 0 {
				maxCtx = caps.MaxContext
			}
			views = append(views, WorkerView{
				ID:           w.ID,
				Endpoint:     endpoint,
				BaseModel:    m.BaseModel,
				Adapter:      m.Adapter,
				Healthy:      live,
				Ready:        ready,
				QueueDepth:   queue,
				InFlight:     inflight,
				MaxContext:   maxCtx,
				CostPerHour:  caps.Cost.PerHourUSD,
				CostClass:    caps.Cost.Class,
				Runtime:      caps.Runtime,
				VRAMGB:       caps.VRAMGB,
				GPU:          caps.GPU,
				Capabilities: caps.Raw,
			})
		}
	}

	return &RoutingSnapshot{
		BuiltAt: time.Now().UTC(),
		Epoch:   p.epoch.Add(1),
		Workers: views,
	}, nil
}
