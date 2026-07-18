package catalog

import "time"

type Model struct {
	ID        string `gorm:"primaryKey;type:text"`
	Name      string `gorm:"uniqueIndex;not null"`
	BaseModel string `gorm:"index;not null"`
	Adapter   string `gorm:"index"`
	CreatedAt time.Time
}

func (Model) TableName() string { return "catalog_models" }

type ModelAssignment struct {
	ID        string `gorm:"primaryKey;type:text"`
	ModelID   string `gorm:"index;not null"`
	WorkerID  string `gorm:"index;not null"`
	CreatedAt time.Time
}

func (ModelAssignment) TableName() string { return "catalog_model_assignments" }
