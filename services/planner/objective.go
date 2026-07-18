package planner

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"
)

type ObjectiveStore struct {
	db  *gorm.DB
	mu  sync.RWMutex
	cur Objective
}

func DefaultObjective() Objective {
	return Objective{
		TargetTTFTMs:    800,
		TargetTPOTMs:    50,
		MaxCostPerHour:  0,
		WeightLoad:      0.5,
		WeightLatency:   0.2,
		WeightCost:      0.2,
		WeightAffinity:  0.1,
		EvalIntervalSec: 45,
		UpdatedAt:       time.Now().UTC(),
	}
}

func NewObjectiveStore(db *gorm.DB) *ObjectiveStore {
	s := &ObjectiveStore{db: db, cur: DefaultObjective()}
	if db != nil {
		_ = s.Reload(context.Background())
	}
	return s
}

func (s *ObjectiveStore) Get() Objective {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

func (s *ObjectiveStore) Set(o Objective) {
	s.mu.Lock()
	s.cur = o
	s.mu.Unlock()
}

func (s *ObjectiveStore) Reload(ctx context.Context) error {
	if s.db == nil {
		return nil
	}
	var o Objective
	err := s.db.WithContext(ctx).Order("id desc").First(&o).Error
	if err == gorm.ErrRecordNotFound {
		o = DefaultObjective()
		if err := s.db.WithContext(ctx).Create(&o).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	s.Set(o)
	return nil
}

func (s *ObjectiveStore) Upsert(ctx context.Context, o Objective) error {
	if s.db == nil {
		s.Set(o)
		return nil
	}
	o.UpdatedAt = time.Now().UTC()
	if o.ID == 0 {
		var existing Objective
		if err := s.db.WithContext(ctx).Order("id desc").First(&existing).Error; err == nil {
			o.ID = existing.ID
		}
	}
	if err := s.db.WithContext(ctx).Save(&o).Error; err != nil {
		return err
	}
	s.Set(o)
	return nil
}
