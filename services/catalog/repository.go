package catalog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository interface {
	ListModels(ctx context.Context) ([]Model, error)
	ListAssignments(ctx context.Context) ([]ModelAssignment, error)
	UpsertModel(ctx context.Context, m *Model) error
	UpsertAssignment(ctx context.Context, a *ModelAssignment) error
	SeedFromWorkers(ctx context.Context) (int, error)
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) ListModels(ctx context.Context) ([]Model, error) {
	var models []Model
	err := r.db.WithContext(ctx).Order("name asc").Find(&models).Error
	return models, err
}

func (r *gormRepository) ListAssignments(ctx context.Context) ([]ModelAssignment, error) {
	var rows []ModelAssignment
	err := r.db.WithContext(ctx).Find(&rows).Error
	return rows, err
}

func (r *gormRepository) UpsertModel(ctx context.Context, m *Model) error {
	if m.ID == "" {
		m.ID = newID()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"base_model", "adapter"}),
	}).Create(m).Error
}

func (r *gormRepository) UpsertAssignment(ctx context.Context, a *ModelAssignment) error {
	if a.ID == "" {
		a.ID = newID()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "model_id"}, {Name: "worker_id"}},
		DoNothing: true,
	}).Create(a).Error
}

func (r *gormRepository) SeedFromWorkers(ctx context.Context) (int, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Model{}).Count(&count).Error; err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, nil
	}

	type row struct {
		WorkerID  string
		BaseModel string
		Adapter   string
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("servable_models").
		Select("worker_id, base_model, COALESCE(adapter, '') as adapter").
		Find(&rows).Error; err != nil {
		return 0, err
	}

	n := 0
	for _, rw := range rows {
		if rw.BaseModel == "" {
			continue
		}
		name := rw.BaseModel
		if rw.Adapter != "" {
			name = rw.BaseModel + "#" + rw.Adapter
		}
		m := &Model{
			ID:        newID(),
			Name:      name,
			BaseModel: rw.BaseModel,
			Adapter:   rw.Adapter,
			CreatedAt: time.Now().UTC(),
		}
		if err := r.UpsertModel(ctx, m); err != nil {
			return n, fmt.Errorf("seed model %s: %w", name, err)
		}
		var stored Model
		if err := r.db.WithContext(ctx).Where("name = ?", name).First(&stored).Error; err != nil {
			return n, err
		}
		a := &ModelAssignment{
			ID:        newID(),
			ModelID:   stored.ID,
			WorkerID:  rw.WorkerID,
			CreatedAt: time.Now().UTC(),
		}
		if err := r.UpsertAssignment(ctx, a); err != nil {
			return n, fmt.Errorf("seed assignment %s->%s: %w", name, rw.WorkerID, err)
		}
		n++
	}
	return n, nil
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
