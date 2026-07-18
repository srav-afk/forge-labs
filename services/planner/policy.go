package planner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	PolicyKey     = "forge:policy:routing:v1"
	PolicyChannel = "forge:policy:updates"
)

type RoutingPolicy struct {
	Version        uint64                 `json:"version"`
	GeneratedAt    time.Time              `json:"generated_at"`
	ObjectiveHash  string                 `json:"objective_hash"`
	WeightLoad     float64                `json:"weight_load"`
	WeightLatency  float64                `json:"weight_latency"`
	WeightCost     float64                `json:"weight_cost"`
	WeightAffinity float64                `json:"weight_affinity"`
	Models         map[string]ModelPolicy `json:"models"`
	Reason         string                 `json:"reason"`
}

type ModelPolicy struct {
	Weights           map[string]float64 `json:"weights"`
	Affinity          string             `json:"affinity"`
	ConcurrencyTarget int                `json:"concurrency_target"`
	MaxQueueDepth     int                `json:"max_queue_depth"`
}

type PolicyStore struct {
	db  *gorm.DB
	rdb *redis.Client
}

func NewPolicyStore(db *gorm.DB, rdb *redis.Client) *PolicyStore {
	return &PolicyStore{db: db, rdb: rdb}
}

func HashObjective(o Objective) string {
	raw := fmt.Sprintf("%.0f|%.0f|%.2f|%.3f|%.3f|%.3f|%.3f",
		o.TargetTTFTMs, o.TargetTPOTMs, o.MaxCostPerHour,
		o.WeightLoad, o.WeightLatency, o.WeightCost, o.WeightAffinity)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:8])
}

func (s *PolicyStore) Publish(ctx context.Context, p RoutingPolicy, score float64) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if s.db != nil {
		row := Decision{
			Version:       p.Version,
			ObjectiveHash: p.ObjectiveHash,
			PolicyJSON:    datatypes.JSON(body),
			WinningScore:  score,
			Reason:        p.Reason,
			CreatedAt:     time.Now().UTC(),
		}
		if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
	}
	if s.rdb != nil {
		if err := s.rdb.Set(ctx, PolicyKey, body, 0).Err(); err != nil {
			return err
		}
		if err := s.rdb.Publish(ctx, PolicyChannel, fmt.Sprintf("%d", p.Version)).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (s *PolicyStore) LoadLive(ctx context.Context) (*RoutingPolicy, error) {
	if s.rdb == nil {
		return nil, nil
	}
	raw, err := s.rdb.Get(ctx, PolicyKey).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var p RoutingPolicy
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
