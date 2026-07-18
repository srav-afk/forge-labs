package gateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/srav-afk/forge-labs/services/routing"
	"github.com/srav-afk/forge-labs/services/scheduler"
)

var ErrNoSnapshot = errors.New("no snapshot yet")

type SnapshotSelector struct {
	holder         *routing.SnapshotHolder
	inflight       *routing.InflightTracker
	latency        *scheduler.LatencyStore
	chain          *scheduler.Chain
	metrics        *scheduler.Metrics
	admissionLimit int64
}

func NewSnapshotSelector(
	holder *routing.SnapshotHolder,
	inflight *routing.InflightTracker,
	latency *scheduler.LatencyStore,
	chain *scheduler.Chain,
	metrics *scheduler.Metrics,
	admissionLimit int,
) *SnapshotSelector {
	return &SnapshotSelector{
		holder:         holder,
		inflight:       inflight,
		latency:        latency,
		chain:          chain,
		metrics:        metrics,
		admissionLimit: int64(admissionLimit),
	}
}

func (s *SnapshotSelector) SelectWorker(model, prompt string) (*SelectedWorker, error) {
	snap := s.holder.Load()
	if snap == nil {
		return nil, ErrNoSnapshot
	}

	base, adapter := ParseModelID(model)
	req := &scheduler.Request{BaseModel: base, Adapter: adapter, Prompt: prompt}
	candidates := scheduler.CandidatesFromSnapshot(snap, s.inflight, s.latency)

	pick, err := s.chain.PickWithMetrics(context.Background(), req, candidates, s.metrics)
	if err != nil {
		if errors.Is(err, scheduler.ErrAdmissionRejected) {
			return nil, fmt.Errorf("%w: model %q", scheduler.ErrAdmissionRejected, model)
		}
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

func (s *SnapshotSelector) AdmissionLimit() int64 {
	return s.admissionLimit
}
