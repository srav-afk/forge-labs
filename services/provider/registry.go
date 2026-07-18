package provider

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/srav-afk/forge-labs/services/routing"
)

type VirtualWorker struct {
	ID            string
	ProviderID    string
	Endpoint      string
	BaseModel     string
	Adapter       string
	MaxContext    uint32
	CostPerHour   float64
	CostInPerM    float64
	CostOutPerM   float64
	ProviderModel string
	Backend       *OpenAICompat
}

type Registry struct {
	db      *gorm.DB
	mu      sync.RWMutex
	workers []VirtualWorker
	byID    map[string]*OpenAICompat
}

func NewRegistry(db *gorm.DB) *Registry {
	return &Registry{db: db, byID: map[string]*OpenAICompat{}}
}

func (r *Registry) Reload(ctx context.Context) error {
	if r.db == nil {
		return nil
	}
	var providers []Provider
	if err := r.db.WithContext(ctx).Where("enabled = ?", true).Find(&providers).Error; err != nil {
		return err
	}
	var maps []ModelMap
	if err := r.db.WithContext(ctx).Find(&maps).Error; err != nil {
		return err
	}
	byProv := map[string][]ModelMap{}
	for _, m := range maps {
		byProv[m.ProviderID] = append(byProv[m.ProviderID], m)
	}

	workers := []VirtualWorker{}
	backends := map[string]*OpenAICompat{}
	for _, p := range providers {
		modelMap := map[string]string{}
		for _, m := range byProv[p.ID] {
			modelMap[m.BaseModel] = m.ProviderModel
		}
		backend := NewOpenAICompat(OpenAIConfig{
			ID:      p.ID,
			BaseURL: p.BaseURL,
			APIKey:  ResolveAPIKey(p.APIKeyRef),
			Models:  modelMap,
		})
		backends[p.ID] = backend
		costHr := p.CostPerHour
		if costHr <= 0 {
			costHr = (p.CostInPerMTok + p.CostOutPerMTok) * 10
			if costHr <= 0 {
				costHr = 1.0
			}
		}
		for _, m := range byProv[p.ID] {
			wid := fmt.Sprintf("provider:%s:%s", p.ID, m.BaseModel)
			if m.Adapter != "" {
				wid += "#" + m.Adapter
			}
			workers = append(workers, VirtualWorker{
				ID:            wid,
				ProviderID:    p.ID,
				Endpoint:      "provider://" + p.ID,
				BaseModel:     m.BaseModel,
				Adapter:       m.Adapter,
				MaxContext:    uint32(m.MaxContext),
				CostPerHour:   costHr,
				CostInPerM:    p.CostInPerMTok,
				CostOutPerM:   p.CostOutPerMTok,
				ProviderModel: m.ProviderModel,
				Backend:       backend,
			})
		}
	}
	r.mu.Lock()
	r.workers = workers
	r.byID = backends
	r.mu.Unlock()
	return nil
}

func (r *Registry) Workers() []VirtualWorker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]VirtualWorker, len(r.workers))
	copy(out, r.workers)
	return out
}

func (r *Registry) Backend(workerID string) (*OpenAICompat, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, w := range r.workers {
		if w.ID == workerID && w.Backend != nil {
			return w.Backend, true
		}
	}
	return nil, false
}

func (r *Registry) BackendByEndpoint(endpoint string) (*OpenAICompat, bool) {
	if len(endpoint) < 11 || endpoint[:11] != "provider://" {
		return nil, false
	}
	id := endpoint[11:]
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.byID[id]
	return b, ok
}

func (r *Registry) SnapshotViews() []routing.WorkerView {
	now := time.Now()
	_ = now
	views := []routing.WorkerView{}
	for _, w := range r.Workers() {
		views = append(views, routing.WorkerView{
			ID:          w.ID,
			Endpoint:    w.Endpoint,
			BaseModel:   w.BaseModel,
			Adapter:     w.Adapter,
			Healthy:     true,
			Ready:       true,
			MaxContext:  w.MaxContext,
			CostPerHour: w.CostPerHour,
			CostClass:   "paid",
			Runtime:     "provider",
			Capabilities: map[string]string{
				"provider":      "true",
				"no_local_kv":   "true",
				"provider_id":   w.ProviderID,
				"cost_class":    "paid",
				"cost_per_hour": fmt.Sprintf("%.4f", w.CostPerHour),
			},
		})
	}
	return views
}

func (r *Registry) Start(ctx context.Context) {
	_ = r.Reload(ctx)
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = r.Reload(ctx)
			}
		}
	}()
}
