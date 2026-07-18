package scheduler

import "context"

type LeastLoaded struct{}

func (LeastLoaded) Name() string { return "least-loaded" }

func (LeastLoaded) Score(_ context.Context, _ *Request, c Candidate) float64 {
	return 1.0 / float64(1+c.QueueDepth)
}
