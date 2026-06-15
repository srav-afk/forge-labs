package models

import (
	"time"

	"gorm.io/datatypes"
)

type Worker struct {
	ID           string          `gorm:"primaryKey"`
	Endpoint     string          `gorm:"not null"`
	RuntimeKind  string          `gorm:"index;not null"`
	Capabilities datatypes.JSON  `gorm:"type:jsonb"`
	Models       []ServableModel `gorm:"foreignKey:WorkerID;constraint:OnDelete:CASCADE"`
	RegisteredAt time.Time
	UpdatedAt    time.Time
}

type ServableModel struct {
	ID         uint   `gorm:"primaryKey"`
	WorkerID   string `gorm:"index;not null"`
	BaseModel  string `gorm:"index;not null"`
	Adapter    string `gorm:"index"`
	MaxContext uint32
}
