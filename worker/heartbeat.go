package worker

import (
	"context"
	"encoding/json"
	"log"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/srav-afk/forge-labs/services/health"
)

type HeartbeatWriter struct {
	rdb       *redis.Client
	id        string
	baseModel string
	adapter   *string
	runtime   string
	addr      string
	ttl       time.Duration
	interval  time.Duration
	ready     atomic.Bool
	inflight  atomic.Int64
	queue     atomic.Int64
}

type HeartbeatWriterConfig struct {
	RDB       *redis.Client
	ID        string
	BaseModel string
	Adapter   *string
	Runtime   string
	Addr      string
	TTL       time.Duration
	Interval  time.Duration
	Ready     bool
}

func NewHeartbeatWriter(cfg HeartbeatWriterConfig) *HeartbeatWriter {
	if cfg.TTL <= 0 {
		cfg.TTL = 6 * time.Second
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 2 * time.Second
	}
	w := &HeartbeatWriter{
		rdb:       cfg.RDB,
		id:        cfg.ID,
		baseModel: cfg.BaseModel,
		adapter:   cfg.Adapter,
		runtime:   cfg.Runtime,
		addr:      cfg.Addr,
		ttl:       cfg.TTL,
		interval:  cfg.Interval,
	}
	w.ready.Store(cfg.Ready)
	return w
}

func (w *HeartbeatWriter) SetReady(ready bool) {
	w.ready.Store(ready)
}

func (w *HeartbeatWriter) SetLoad(inflight, queueDepth int64) {
	w.inflight.Store(inflight)
	w.queue.Store(queueDepth)
}

func (w *HeartbeatWriter) snapshot() health.Heartbeat {
	return health.Heartbeat{
		ID:         w.id,
		BaseModel:  w.baseModel,
		Adapter:    w.adapter,
		Runtime:    w.runtime,
		Addr:       w.addr,
		Ready:      w.ready.Load(),
		Inflight:   int(w.inflight.Load()),
		QueueDepth: int(w.queue.Load()),
		TS:         time.Now().UnixMilli(),
	}
}

func (w *HeartbeatWriter) BeatOnce(ctx context.Context) error {
	payload, err := json.Marshal(w.snapshot())
	if err != nil {
		return err
	}
	return w.rdb.Set(ctx, health.Key(w.id), payload, w.ttl).Err()
}

func (w *HeartbeatWriter) Run(ctx context.Context) {
	if err := w.BeatOnce(ctx); err != nil {
		log.Printf("heartbeat: initial beat failed: %v", err)
	}

	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := w.BeatOnce(ctx); err != nil {
				log.Printf("heartbeat: beat failed: %v", err)
			}
		}
	}
}

func (w *HeartbeatWriter) Delete(ctx context.Context) error {
	return w.rdb.Del(ctx, health.Key(w.id)).Err()
}
