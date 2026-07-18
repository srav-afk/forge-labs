package gateway

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/srav-afk/forge-labs/services/registry"
	"github.com/srav-afk/forge-labs/services/registry/models"
)

type SelectedWorker struct {
	ID       string
	Endpoint string
	Models   []string
}

type WorkerSelector interface {
	SelectWorker(model string) (*SelectedWorker, error)
	ListModels() []modelObject
}

type registrySelector struct {
	repo     registry.WorkerRepository
	mu       sync.RWMutex
	workers  []SelectedWorker
	fallback string
}

func NewRegistrySelector(repo registry.WorkerRepository, fallbackEndpoint string) *registrySelector {
	s := &registrySelector{repo: repo, fallback: fallbackEndpoint}
	_ = s.refresh(context.Background())
	return s
}

func (s *registrySelector) Start(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = 5 * time.Second
	}
	t := time.NewTicker(every)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = s.refresh(ctx)
			}
		}
	}()
}

func (s *registrySelector) refresh(ctx context.Context) error {
	workers, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	out := make([]SelectedWorker, 0, len(workers))
	for _, w := range workers {
		out = append(out, toSelected(w))
	}
	s.mu.Lock()
	s.workers = out
	s.mu.Unlock()
	return nil
}

func toSelected(w models.Worker) SelectedWorker {
	modelsIDs := make([]string, 0, len(w.Models))
	for _, m := range w.Models {
		id := m.BaseModel
		if m.Adapter != "" {
			id = m.BaseModel + "#" + m.Adapter
		}
		modelsIDs = append(modelsIDs, id)
	}
	return SelectedWorker{
		ID:       w.ID,
		Endpoint: w.Endpoint,
		Models:   modelsIDs,
	}
}

func (s *registrySelector) SelectWorker(model string) (*SelectedWorker, error) {
	base, _ := ParseModelID(model)
	s.mu.RLock()
	workers := s.workers
	s.mu.RUnlock()

	if len(workers) == 0 && s.fallback != "" {
		return &SelectedWorker{
			ID:       "fallback",
			Endpoint: s.fallback,
			Models:   []string{model},
		}, nil
	}
	if len(workers) == 0 {
		return nil, fmt.Errorf("no workers registered")
	}

	for i := range workers {
		w := workers[i]
		for _, m := range w.Models {
			mb, _ := ParseModelID(m)
			if m == model || mb == base || mb == model {
				cp := w
				return &cp, nil
			}
		}
	}

	if len(workers) == 1 {
		cp := workers[0]
		return &cp, nil
	}
	return nil, fmt.Errorf("no worker for model %q", model)
}

func (s *registrySelector) ListModels() []modelObject {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]struct{}{}
	var out []modelObject
	now := time.Now().Unix()
	for _, w := range s.workers {
		for _, id := range w.Models {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, modelObject{
				ID:      id,
				Object:  "model",
				Created: now,
				OwnedBy: "forge",
			})
		}
	}
	return out
}
