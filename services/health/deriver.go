package health

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Service struct {
	rdb       *redis.Client
	snapshot  *Snapshot
	metrics   *Metrics
	reconcile time.Duration
}

func NewService(rdb *redis.Client, metrics *Metrics, reconcileInterval time.Duration) *Service {
	if reconcileInterval <= 0 {
		reconcileInterval = 3 * time.Second
	}
	return &Service{
		rdb:       rdb,
		snapshot:  NewSnapshot(),
		metrics:   metrics,
		reconcile: reconcileInterval,
	}
}

func (s *Service) Snapshot() *Snapshot {
	return s.snapshot
}

func (s *Service) Routable() []Heartbeat {
	return s.snapshot.Routable()
}

func (s *Service) Start(ctx context.Context) {
	if err := s.reconcileOnce(ctx); err != nil {
		log.Printf("health: initial reconcile: %v", err)
	}
	s.publish()

	go s.listenKeyevents(ctx)
	go s.reconcileLoop(ctx)
}

func (s *Service) reconcileLoop(ctx context.Context) {
	t := time.NewTicker(s.reconcile)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.reconcileOnce(ctx); err != nil {
				log.Printf("health: reconcile: %v", err)
				continue
			}
			s.publish()
		}
	}
}

func (s *Service) reconcileOnce(ctx context.Context) error {
	live := make(map[string]Heartbeat)
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, keyGlob, 100).Result()
		if err != nil {
			return err
		}
		for _, k := range keys {
			val, err := s.rdb.Get(ctx, k).Result()
			if errors.Is(err, redis.Nil) {
				continue
			}
			if err != nil {
				return err
			}
			var hb Heartbeat
			if err := json.Unmarshal([]byte(val), &hb); err != nil {
				log.Printf("health: bad heartbeat payload on %s: %v", k, err)
				continue
			}
			if hb.ID == "" {
				if id, ok := WorkerIDFromKey(k); ok {
					hb.ID = id
				} else {
					continue
				}
			}
			live[hb.ID] = hb
		}
		if next == 0 {
			break
		}
		cursor = next
	}
	s.snapshot.Replace(live)
	return nil
}

func (s *Service) listenKeyevents(ctx context.Context) {
	pubsub := s.rdb.PSubscribe(ctx,
		"__keyevent@0__:expired",
		"__keyevent@0__:del",
		"__keyevent@0__:set",
	)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			id, ok := WorkerIDFromKey(msg.Payload)
			if !ok {
				continue
			}
			switch {
			case strings.HasSuffix(msg.Channel, ":expired"), strings.HasSuffix(msg.Channel, ":del"):
				s.snapshot.Remove(id)
				s.publish()
			case strings.HasSuffix(msg.Channel, ":set"):
				s.applySet(ctx, id)
			}
		}
	}
}

func (s *Service) applySet(ctx context.Context, id string) {
	val, err := s.rdb.Get(ctx, Key(id)).Result()
	if errors.Is(err, redis.Nil) {
		s.snapshot.Remove(id)
		s.publish()
		return
	}
	if err != nil {
		log.Printf("health: get after set for %s: %v", id, err)
		return
	}
	var hb Heartbeat
	if err := json.Unmarshal([]byte(val), &hb); err != nil {
		log.Printf("health: bad heartbeat on set for %s: %v", id, err)
		return
	}
	if hb.ID == "" {
		hb.ID = id
	}
	s.snapshot.Upsert(hb)
	s.publish()
}

func (s *Service) publish() {
	s.metrics.Publish(s.snapshot.All())
}
