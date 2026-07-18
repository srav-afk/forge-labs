package fleet

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"
)

type ScalingPolicy struct {
	ID                         uint   `gorm:"primaryKey"`
	BaseModel                  string `gorm:"uniqueIndex:idx_fleet_model_identity;not null"`
	Adapter                    string `gorm:"uniqueIndex:idx_fleet_model_identity"`
	MinReplicas                int
	MaxReplicas                int
	TargetConcurrency          int
	ScaleUpUtilization         float64
	ScaleDownDelaySeconds      int
	StabilizationWindowSeconds int
	UpdatedAt                  time.Time
}

func (ScalingPolicy) TableName() string { return "fleet_scaling_policies" }

type ModelIdentity struct {
	BaseModel string
	Adapter   string
}

func (m ModelIdentity) Key() string {
	if m.Adapter == "" {
		return m.BaseModel
	}
	return m.BaseModel + "#" + m.Adapter
}

type PolicyCache struct {
	db *gorm.DB
	mu sync.RWMutex
	by map[string]ScalingPolicy
}

func NewPolicyCache(db *gorm.DB) *PolicyCache {
	return &PolicyCache{db: db, by: map[string]ScalingPolicy{}}
}

func DefaultPolicy(id ModelIdentity) ScalingPolicy {
	return ScalingPolicy{
		BaseModel:                  id.BaseModel,
		Adapter:                    id.Adapter,
		MinReplicas:                0,
		MaxReplicas:                3,
		TargetConcurrency:          16,
		ScaleUpUtilization:         0.70,
		ScaleDownDelaySeconds:      900,
		StabilizationWindowSeconds: 60,
		UpdatedAt:                  time.Now().UTC(),
	}
}

func (c *PolicyCache) Get(id ModelIdentity) ScalingPolicy {
	c.mu.RLock()
	p, ok := c.by[id.Key()]
	c.mu.RUnlock()
	if ok {
		return p
	}
	return DefaultPolicy(id)
}

func (c *PolicyCache) Reload(ctx context.Context) error {
	if c.db == nil {
		return nil
	}
	var rows []ScalingPolicy
	if err := c.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return err
	}
	next := map[string]ScalingPolicy{}
	for _, r := range rows {
		next[ModelIdentity{BaseModel: r.BaseModel, Adapter: r.Adapter}.Key()] = r
	}
	c.mu.Lock()
	c.by = next
	c.mu.Unlock()
	return nil
}

func (c *PolicyCache) Upsert(ctx context.Context, p ScalingPolicy) error {
	p.UpdatedAt = time.Now().UTC()
	if c.db != nil {
		if err := c.db.WithContext(ctx).Where("base_model = ? AND adapter = ?", p.BaseModel, p.Adapter).
			Assign(p).FirstOrCreate(&p).Error; err != nil {
			return err
		}
	}
	c.mu.Lock()
	c.by[ModelIdentity{BaseModel: p.BaseModel, Adapter: p.Adapter}.Key()] = p
	c.mu.Unlock()
	return nil
}
