package catalog

import (
	"context"
	"encoding/json"
	"log"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const Channel = "forge:models:changed"

type ModelIdentity struct {
	ID        string
	Name      string
	BaseModel string
	Adapter   string
}

type Snapshot struct {
	ByName         map[string]ModelIdentity
	WorkersByModel map[string][]string
	BuiltAt        time.Time
}

type SnapshotHolder struct {
	ptr atomic.Pointer[Snapshot]
}

func NewSnapshotHolder() *SnapshotHolder {
	return &SnapshotHolder{}
}

func (h *SnapshotHolder) Load() *Snapshot {
	return h.ptr.Load()
}

func (h *SnapshotHolder) Store(s *Snapshot) {
	if s == nil {
		return
	}
	h.ptr.Store(s)
}

func (s *Snapshot) Resolve(name string) (ModelIdentity, []string, bool) {
	if s == nil {
		return ModelIdentity{}, nil, false
	}
	id, ok := s.ByName[name]
	if !ok {
		return ModelIdentity{}, nil, false
	}
	workers := s.WorkersByModel[id.ID]
	return id, workers, true
}

func (s *Snapshot) Empty() bool {
	return s == nil || len(s.ByName) == 0
}

func (s *Snapshot) ModelNames() []ModelIdentity {
	if s == nil {
		return nil
	}
	out := make([]ModelIdentity, 0, len(s.ByName))
	for _, m := range s.ByName {
		out = append(out, m)
	}
	return out
}

type Service struct {
	repo   Repository
	holder *SnapshotHolder
	rdb    *redis.Client
}

func NewService(repo Repository, holder *SnapshotHolder, rdb *redis.Client) *Service {
	return &Service{repo: repo, holder: holder, rdb: rdb}
}

func (s *Service) Holder() *SnapshotHolder { return s.holder }

func (s *Service) Rebuild(ctx context.Context) error {
	if _, err := s.repo.SeedFromWorkers(ctx); err != nil {
		log.Printf("catalog: seed: %v", err)
	}
	models, err := s.repo.ListModels(ctx)
	if err != nil {
		return err
	}
	assigns, err := s.repo.ListAssignments(ctx)
	if err != nil {
		return err
	}

	byName := make(map[string]ModelIdentity, len(models))
	workersByModel := make(map[string][]string)
	for _, m := range models {
		byName[m.Name] = ModelIdentity{
			ID:        m.ID,
			Name:      m.Name,
			BaseModel: m.BaseModel,
			Adapter:   m.Adapter,
		}
	}
	for _, a := range assigns {
		workersByModel[a.ModelID] = append(workersByModel[a.ModelID], a.WorkerID)
	}
	s.holder.Store(&Snapshot{
		ByName:         byName,
		WorkersByModel: workersByModel,
		BuiltAt:        time.Now().UTC(),
	})
	return nil
}

func (s *Service) PublishChanged(ctx context.Context, reason string) error {
	if s.rdb == nil {
		return nil
	}
	payload, _ := json.Marshal(map[string]string{"reason": reason})
	return s.rdb.Publish(ctx, Channel, payload).Err()
}

func (s *Service) Start(ctx context.Context) {
	if err := s.Rebuild(ctx); err != nil {
		log.Printf("catalog: initial rebuild: %v", err)
	}
	go s.subscribe(ctx)
	// periodic rebuild as belt-and-suspenders (worker registration may lag seed)
	t := time.NewTicker(5 * time.Second)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := s.Rebuild(ctx); err != nil {
					log.Printf("catalog: rebuild: %v", err)
				}
			}
		}
	}()
}

func (s *Service) subscribe(ctx context.Context) {
	if s.rdb == nil {
		return
	}
	sub := s.rdb.Subscribe(ctx, Channel)
	defer sub.Close()
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_ = msg
			if err := s.Rebuild(ctx); err != nil {
				log.Printf("catalog: rebuild on change: %v", err)
			}
		}
	}
}
