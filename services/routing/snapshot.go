package routing

import "time"

const Channel = "forge:routing:snapshot"

type WorkerView struct {
	ID           string            `json:"ID"`
	Endpoint     string            `json:"Endpoint"`
	BaseModel    string            `json:"BaseModel"`
	Adapter      string            `json:"Adapter"`
	Healthy      bool              `json:"Healthy"`
	Ready        bool              `json:"Ready"`
	QueueDepth   int               `json:"QueueDepth"`
	InFlight     int               `json:"InFlight"`
	MaxContext   uint32            `json:"MaxContext,omitempty"`
	CostPerHour  float64           `json:"CostPerHour,omitempty"`
	CostClass    string            `json:"CostClass,omitempty"`
	Runtime      string            `json:"Runtime,omitempty"`
	VRAMGB       float64           `json:"VRAMGB,omitempty"`
	GPU          string            `json:"GPU,omitempty"`
	Capabilities map[string]string `json:"Capabilities,omitempty"`
}

type RoutingSnapshot struct {
	BuiltAt time.Time    `json:"BuiltAt"`
	Epoch   uint64       `json:"Epoch"`
	Workers []WorkerView `json:"Workers"`
}

func (s *RoutingSnapshot) ModelID(w WorkerView) string {
	if w.Adapter == "" {
		return w.BaseModel
	}
	return w.BaseModel + "#" + w.Adapter
}
