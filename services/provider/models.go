package provider

import "time"

type Provider struct {
	ID             string `gorm:"primaryKey"`
	Kind           string `gorm:"not null"`
	BaseURL        string `gorm:"not null"`
	AuthMode       string
	APIKeyRef      string
	Region         string
	Enabled        bool
	MaxRPM         int
	SpendCapUSDDay float64
	CostInPerMTok  float64
	CostOutPerMTok float64
	CostPerHour    float64
	UpdatedAt      time.Time
}

func (Provider) TableName() string { return "providers" }

type ModelMap struct {
	ProviderID    string `gorm:"primaryKey"`
	BaseModel     string `gorm:"primaryKey"`
	Adapter       string `gorm:"primaryKey"`
	ProviderModel string `gorm:"not null"`
	MaxContext    int
}

func (ModelMap) TableName() string { return "provider_model_map" }
