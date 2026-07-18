package planner

import (
	"time"

	"gorm.io/datatypes"
)

type Objective struct {
	ID              uint `gorm:"primaryKey"`
	TargetTTFTMs    float64
	TargetTPOTMs    float64
	MaxCostPerHour  float64
	WeightLoad      float64
	WeightLatency   float64
	WeightCost      float64
	WeightAffinity  float64
	EvalIntervalSec int
	UpdatedAt       time.Time
}

func (Objective) TableName() string { return "planner_objectives" }

type Decision struct {
	ID            uint           `gorm:"primaryKey"`
	Version       uint64         `gorm:"uniqueIndex;not null"`
	ObjectiveHash string         `gorm:"not null"`
	PolicyJSON    datatypes.JSON `gorm:"type:jsonb"`
	WinningScore  float64
	Reason        string
	CreatedAt     time.Time
}

func (Decision) TableName() string { return "planner_decisions" }
