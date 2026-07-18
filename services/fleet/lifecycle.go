package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/srav-afk/forge-labs/services/catalog"
	"github.com/srav-afk/forge-labs/services/provider"
	"github.com/srav-afk/forge-labs/services/routing"
)

type LifecycleConfig struct {
	ReadyTimeout time.Duration
	PollInterval time.Duration
	HTTPClient   *http.Client
}

type Lifecycle struct {
	inner  Provisioner
	holder *routing.SnapshotHolder
	db     *gorm.DB
	cfg    LifecycleConfig
	onReady func(WorkerID, ModelIdentity)
}

func NewLifecycle(inner Provisioner, holder *routing.SnapshotHolder, db *gorm.DB, cfg LifecycleConfig) *Lifecycle {
	if cfg.ReadyTimeout <= 0 {
		cfg.ReadyTimeout = 10 * time.Minute
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Lifecycle{inner: inner, holder: holder, db: db, cfg: cfg}
}

func (l *Lifecycle) Kind() string { return l.inner.Kind() }

func (l *Lifecycle) OnWorkerReady(fn func(WorkerID, ModelIdentity)) { l.onReady = fn }

func (l *Lifecycle) Provision(ctx context.Context, id ModelIdentity) (WorkerID, error) {
	wid, err := l.inner.Provision(ctx, id)
	if err != nil {
		return "", err
	}
	return wid, nil
}

func (l *Lifecycle) Retire(ctx context.Context, w WorkerID) error {
	return l.inner.Retire(ctx, w)
}

func (l *Lifecycle) WaitReady(ctx context.Context, wid WorkerID, id ModelIdentity) error {
	deadline := time.Now().Add(l.cfg.ReadyTimeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		ready, detail, err := l.checkReady(ctx, wid, id)
		if err != nil {
			log.Printf("fleet/lifecycle: check %s: %v", wid, err)
		}
		if ready {
			if err := l.register(ctx, wid, id, detail); err != nil {
				return err
			}
			if l.onReady != nil {
				l.onReady(wid, id)
			}
			log.Printf("fleet/lifecycle: ready %s model=%s (%s)", wid, id.Key(), detail)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.cfg.PollInterval):
		}
	}
	return fmt.Errorf("fleet/lifecycle: timeout waiting for %s", wid)
}

func (l *Lifecycle) checkReady(ctx context.Context, wid WorkerID, id ModelIdentity) (bool, string, error) {
	if strings.HasPrefix(string(wid), "runpod-") && !strings.HasPrefix(string(wid), "runpod-dryrun-") {
		podID := strings.TrimPrefix(string(wid), "runpod-")
		proxy := fmt.Sprintf("https://%s-8000.proxy.runpod.net", podID)
		ok, err := l.httpModelsOK(ctx, proxy+"/v1/models", id.BaseModel)
		if err != nil {
			return false, proxy, err
		}
		return ok, proxy, nil
	}
	if l.holder != nil {
		if snap := l.holder.Load(); snap != nil {
			for _, w := range snap.Workers {
				if w.ID == string(wid) && w.Healthy && w.Ready {
					return true, "heartbeat", nil
				}
			}
		}
	}
	if strings.HasPrefix(string(wid), "runpod-dryrun-") || strings.HasPrefix(string(wid), "local-") {
		if l.holder == nil {
			return true, "track-only", nil
		}
		if snap := l.holder.Load(); snap != nil {
			for _, w := range snap.Workers {
				if w.ID == string(wid) && w.Healthy && w.Ready {
					return true, "heartbeat", nil
				}
			}
		}
		if strings.HasPrefix(string(wid), "local-") {
			return false, "waiting-heartbeat", nil
		}
		return true, "dry-run", nil
	}
	return false, "", nil
}

func (l *Lifecycle) httpModelsOK(ctx context.Context, url, wantModel string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := l.cfg.HTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		return false, fmt.Errorf("status %d", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return false, err
	}
	if wantModel == "" {
		return len(body.Data) > 0, nil
	}
	for _, m := range body.Data {
		if m.ID == wantModel || strings.Contains(m.ID, wantModel) {
			return true, nil
		}
	}
	return len(body.Data) > 0, nil
}

func (l *Lifecycle) register(ctx context.Context, wid WorkerID, id ModelIdentity, detail string) error {
	if l.db == nil {
		return nil
	}
	if strings.HasPrefix(string(wid), "runpod-") && !strings.HasPrefix(string(wid), "runpod-dryrun-") {
		podID := strings.TrimPrefix(string(wid), "runpod-")
		baseURL := detail
		if !strings.HasSuffix(baseURL, "/v1") {
			baseURL = strings.TrimRight(detail, "/") + "/v1"
		}
		provID := "runpod-" + podID
		p := provider.Provider{
			ID:             provID,
			Kind:           "openai_compat",
			BaseURL:        baseURL,
			AuthMode:       "none",
			APIKeyRef:      "",
			Enabled:        true,
			CostPerHour:    0.39,
			UpdatedAt:      time.Now().UTC(),
		}
		if err := l.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"base_url", "enabled", "updated_at"}),
		}).Create(&p).Error; err != nil {
			return err
		}
		mm := provider.ModelMap{
			ProviderID:    provID,
			BaseModel:     id.BaseModel,
			Adapter:       id.Adapter,
			ProviderModel: id.BaseModel,
			MaxContext:    8192,
		}
		if err := l.db.WithContext(ctx).Save(&mm).Error; err != nil {
			return err
		}
		return ensureCatalogAssignment(ctx, l.db, id, "provider:"+provID+":"+id.BaseModel)
	}
	return ensureCatalogAssignment(ctx, l.db, id, string(wid))
}

func ensureCatalogAssignment(ctx context.Context, db *gorm.DB, id ModelIdentity, workerID string) error {
	if db == nil || workerID == "" || id.BaseModel == "" {
		return nil
	}
	name := id.BaseModel
	if id.Adapter != "" {
		name = id.BaseModel + "#" + id.Adapter
	}
	repo := catalog.NewRepository(db)
	models, err := repo.ListModels(ctx)
	if err != nil {
		return err
	}
	modelID := ""
	for _, m := range models {
		if m.Name == name {
			modelID = m.ID
			break
		}
	}
	if modelID == "" {
		m := &catalog.Model{
			Name:      name,
			BaseModel: id.BaseModel,
			Adapter:   id.Adapter,
			CreatedAt: time.Now().UTC(),
		}
		if err := repo.UpsertModel(ctx, m); err != nil {
			return err
		}
		modelID = m.ID
		if modelID == "" {
			models, _ = repo.ListModels(ctx)
			for _, x := range models {
				if x.Name == name {
					modelID = x.ID
					break
				}
			}
		}
	}
	return repo.UpsertAssignment(ctx, &catalog.ModelAssignment{
		ModelID:   modelID,
		WorkerID:  workerID,
		CreatedAt: time.Now().UTC(),
	})
}
