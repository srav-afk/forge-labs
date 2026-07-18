package scheduler

import "context"

type Candidate struct {
	WorkerID      string
	Endpoint      string
	QueueDepth    int
	Healthy       bool
	Ready         bool
	EwmaLatencyMs float64
	Models        []string
}

type Request struct {
	BaseModel       string
	Adapter         string
	Prompt          string
	AffinityKey     string
	PreferredWorker string
}

type Filter interface {
	Name() string
	Filter(ctx context.Context, req *Request, in []Candidate) []Candidate
}

type Scorer interface {
	Name() string
	Score(ctx context.Context, req *Request, c Candidate) float64
}

type WeightedScorer struct {
	Scorer Scorer
	Weight float64
}
