package gateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/srav-afk/forge-labs/services/routing"
	"github.com/srav-afk/forge-labs/services/scheduler"
)

var (
	ErrNoSnapshot = errors.New("no snapshot yet")
	errNoCapacity = scheduler.ErrNoCapacity
)

type SnapshotSelector struct {
	holder   *routing.SnapshotHolder
	inflight *routing.InflightTracker
	chain    *scheduler.Chain
	metrics  *scheduler.Metrics
}

func NewSnapshotSelector(
	holder *routing.SnapshotHolder,
	inflight *routing.InflightTracker,
	chain *scheduler.Chain,
	metrics *scheduler.Metrics,
) *SnapshotSelector {
	return &SnapshotSelector{
		holder:   holder,
		inflight: inflight,
		chain:    chain,
		metrics:  metrics,
	}
}

func (s *SnapshotSelector) SelectWorker(model string) (*SelectedWorker, error) {
	snap := s.holder.Load()
	if snap == nil {
		return nil, ErrNoSnapshot
	}

	base, adapter := ParseModelID(model)
	req := &scheduler.Request{BaseModel: base, Adapter: adapter}
	candidates := scheduler.CandidatesFromSnapshot(snap, s.inflight)

	pick, err := s.chain.Pick(context.Background(), req, candidates)
	if err != nil {
		if errors.Is(err, scheduler.ErrNoCapacity) {
			return nil, fmt.Errorf("%w: model %q", scheduler.ErrNoCapacity, model)
		}
		return nil, err
	}

	if s.metrics != nil {
		s.metrics.IncDispatched(pick.WorkerID, model)
	}

	return &SelectedWorker{
		ID:       pick.WorkerID,
		Endpoint: pick.Endpoint,
		Models:   []string{model},
	}, nil
}

func (s *SnapshotSelector) ListModels() []modelObject {
	snap := s.holder.Load()
	if snap == nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]modelObject, 0)
	created := snap.BuiltAt.Unix()
	if created == 0 {
		created = time.Now().Unix()
	}
	for _, w := range snap.Workers {
		if w.BaseModel == "" {
			continue
		}
		id := w.BaseModel
		if w.Adapter != "" {
			id = w.BaseModel + "#" + w.Adapter
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, modelObject{
			ID:      id,
			Object:  "model",
			Created: created,
			OwnedBy: "forge",
		})
	}
	return out
}

func (s *SnapshotSelector) Inflight() *routing.InflightTracker {
	return s.inflight
}
