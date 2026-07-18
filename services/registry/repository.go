package registry

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/srav-afk/forge-labs/services/registry/models"
)

type WorkerRepository interface {
	Upsert(ctx context.Context, w *models.Worker) error
	List(ctx context.Context) ([]models.Worker, error)
}

type gormWorkerRepository struct {
	db *gorm.DB
}

func NewWorkerRepository(db *gorm.DB) WorkerRepository {
	return &gormWorkerRepository{db: db}
}

func (r *gormWorkerRepository) Upsert(ctx context.Context, w *models.Worker) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Models").Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"endpoint", "runtime_kind", "capabilities", "updated_at"}),
		}).Create(w).Error; err != nil {
			return err
		}

		if err := tx.Where("worker_id = ?", w.ID).Delete(&models.ServableModel{}).Error; err != nil {
			return err
		}

		if len(w.Models) == 0 {
			return nil
		}

		for i := range w.Models {
			w.Models[i].ID = 0
			w.Models[i].WorkerID = w.ID
		}
		return tx.Create(&w.Models).Error
	})
}

func (r *gormWorkerRepository) List(ctx context.Context) ([]models.Worker, error) {
	var workers []models.Worker
	err := r.db.WithContext(ctx).Preload("Models").Find(&workers).Error
	return workers, err
}
