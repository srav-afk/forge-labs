package cacheregistry

import (
	"context"
	"encoding/json"
	"log"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/srav-afk/forge-labs/internal/observability"
	"github.com/srav-afk/forge-labs/services/routing"
)

const EventsChannel = "forge:cache:events"

type Event struct {
	Type      string   `json:"type"`
	WorkerID  string   `json:"worker_id"`
	BaseModel string   `json:"base_model"`
	Adapter   string   `json:"adapter"`
	Tier      string   `json:"tier"`
	Hashes    []uint64 `json:"hashes"`
}

type Snapshot struct {
	WorkerBlocks map[string]map[uint64]float64
	BuiltAt      time.Time
}

type SnapshotHolder struct {
	ptr atomic.Pointer[Snapshot]
}

func NewSnapshotHolder() *SnapshotHolder { return &SnapshotHolder{} }

func (h *SnapshotHolder) Load() *Snapshot { return h.ptr.Load() }

func (h *SnapshotHolder) Store(s *Snapshot) {
	if s != nil {
		h.ptr.Store(s)
	}
}

type Service struct {
	index  *Index
	holder *SnapshotHolder
	rdb    *redis.Client
	events *prometheus.CounterVec
}

func NewService(rdb *redis.Client, reg *observability.Registry) *Service {
	s := &Service{
		index:  NewIndex(rdb),
		holder: NewSnapshotHolder(),
		rdb:    rdb,
		events: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forge_cache_events_total",
			Help: "KV cache registry events ingested",
		}, []string{"type"}),
	}
	if reg != nil {
		reg.MustRegister(s.events)
	}
	s.publishSnapshot()
	return s
}

func (s *Service) Holder() *SnapshotHolder { return s.holder }
func (s *Service) Index() *Index           { return s.index }

func (s *Service) Start(ctx context.Context) {
	go s.subscribe(ctx)
	t := time.NewTicker(2 * time.Second)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.publishSnapshot()
			}
		}
	}()
}

func (s *Service) Apply(ev Event) {
	switch ev.Type {
	case "stored":
		s.index.Store(ev.BaseModel, ev.Adapter, ev.WorkerID, ev.Tier, ev.Hashes)
		if s.events != nil {
			s.events.WithLabelValues("stored").Inc()
		}
	case "removed":
		s.index.Remove(ev.WorkerID, ev.Hashes)
		if s.events != nil {
			s.events.WithLabelValues("removed").Inc()
		}
	case "cleared":
		s.index.ClearWorker(ev.WorkerID)
		if s.events != nil {
			s.events.WithLabelValues("cleared").Inc()
		}
	}
	s.publishSnapshot()
}

func (s *Service) Report(ctx context.Context, workerID, base, adapter, tier string, hashes []uint64) {
	s.Apply(Event{Type: "stored", WorkerID: workerID, BaseModel: base, Adapter: adapter, Tier: tier, Hashes: hashes})
	s.index.Persist(ctx, base, adapter, workerID, hashes)
	if s.rdb != nil {
		body, _ := json.Marshal(Event{Type: "stored", WorkerID: workerID, BaseModel: base, Adapter: adapter, Tier: tier, Hashes: hashes})
		_ = s.rdb.Publish(ctx, EventsChannel, body).Err()
	}
}

func (s *Service) publishSnapshot() {
	s.holder.Store(&Snapshot{
		WorkerBlocks: s.index.SnapshotForScoring(),
		BuiltAt:      time.Now().UTC(),
	})
}

func (s *Service) subscribe(ctx context.Context) {
	if s.rdb == nil {
		return
	}
	sub := s.rdb.Subscribe(ctx, EventsChannel)
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-sub.Channel():
			if !ok {
				return
			}
			var ev Event
			if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
				log.Printf("cache: bad event: %v", err)
				continue
			}
			s.Apply(ev)
		}
	}
}

func InjectIntoRouting(snap *routing.RoutingSnapshot, cache *Snapshot) {
	if snap == nil || cache == nil {
		return
	}
	for i := range snap.Workers {
		if blocks, ok := cache.WorkerBlocks[snap.Workers[i].ID]; ok && len(blocks) > 0 {
			if snap.Workers[i].Capabilities == nil {
				snap.Workers[i].Capabilities = map[string]string{}
			}
			snap.Workers[i].Capabilities["cache_blocks"] = "1"
		}
	}
}
