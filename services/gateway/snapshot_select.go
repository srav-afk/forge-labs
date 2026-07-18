package gateway

import (
	"errors"
	"fmt"
	"time"

	"github.com/srav-afk/forge-labs/services/routing"
)

var ErrNoSnapshot = errors.New("no snapshot yet")

type SnapshotSelector struct {
	holder *routing.SnapshotHolder
}

func NewSnapshotSelector(holder *routing.SnapshotHolder) *SnapshotSelector {
	return &SnapshotSelector{holder: holder}
}

func (s *SnapshotSelector) SelectWorker(model string) (*SelectedWorker, error) {
	snap := s.holder.Load()
	if snap == nil {
		return nil, ErrNoSnapshot
	}

	base, adapter := ParseModelID(model)
	var (
		match   *routing.WorkerView
		anyLive *routing.WorkerView
	)
	for i := range snap.Workers {
		w := &snap.Workers[i]
		if !w.Healthy {
			continue
		}
		if anyLive == nil {
			anyLive = w
		}
		if modelMatches(w, base, adapter, model) {
			if w.Ready || match == nil {
				match = w
				if w.Ready {
					break
				}
			}
		}
	}
	if match == nil && anyLive != nil && len(snap.Workers) == 1 {
		match = anyLive
	}
	if match == nil {
		return nil, fmt.Errorf("no worker for model %q", model)
	}
	return selectedFromView(match), nil
}

func modelMatches(w *routing.WorkerView, base, adapter, model string) bool {
	id := w.BaseModel
	if w.Adapter != "" {
		id = w.BaseModel + "#" + w.Adapter
	}
	if id == model {
		return true
	}
	if w.BaseModel == base && (adapter == "" || w.Adapter == adapter) {
		return true
	}
	return w.BaseModel == model
}

func selectedFromView(w *routing.WorkerView) *SelectedWorker {
	models := []string{}
	if w.BaseModel != "" {
		id := w.BaseModel
		if w.Adapter != "" {
			id = w.BaseModel + "#" + w.Adapter
		}
		models = append(models, id)
	}
	return &SelectedWorker{
		ID:       w.ID,
		Endpoint: w.Endpoint,
		Models:   models,
	}
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
