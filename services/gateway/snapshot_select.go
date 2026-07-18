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
	ws, err := s.SelectWorkers(model, prompt, 1)
	if err != nil {
		return nil, err
	}
	return &ws[0], nil
}

func (s *SnapshotSelector) SelectWorkers(model, prompt string, limit int) ([]SelectedWorker, error) {
	snap := s.holder.Load()
	if snap == nil {
		return nil, ErrNoSnapshot
	}
	if limit <= 0 {
		limit = 3
	}

	base, adapter := ParseModelID(model)
	req := &scheduler.Request{BaseModel: base, Adapter: adapter, Prompt: prompt}
	candidates := scheduler.CandidatesFromSnapshot(snap, s.inflight, s.latency)

	ranked, err := s.chain.Rank(context.Background(), req, candidates)
	if err != nil {
		if errors.Is(err, scheduler.ErrAdmissionRejected) {
			return nil, fmt.Errorf("%w: model %q", scheduler.ErrAdmissionRejected, model)
		}
		if errors.Is(err, scheduler.ErrNoCapacity) {
			return nil, fmt.Errorf("%w: model %q", scheduler.ErrNoCapacity, model)
		}
		return nil, err
	}
	if s.metrics != nil && len(ranked) > 0 {
		s.metrics.SetScore(ranked[0].WorkerID, ranked[0].Score)
		s.metrics.IncRoutingDecision()
		if req.PreferredWorker != "" && ranked[0].WorkerID == req.PreferredWorker {
			s.metrics.IncAffinityHit()
		}
		s.metrics.IncDispatched(ranked[0].WorkerID, model)
	}
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]SelectedWorker, 0, len(ranked))
	for _, p := range ranked {
		out = append(out, SelectedWorker{
			ID:       p.WorkerID,
			Endpoint: p.Endpoint,
			Models:   []string{model},
		})
	}
	return out, nil
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
