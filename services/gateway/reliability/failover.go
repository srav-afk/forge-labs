package reliability

import (
	"context"
	"errors"
	"fmt"
)

type Worker struct {
	ID       string
	Endpoint string
}

type CallFunc func(ctx context.Context, w Worker) error

type Failover struct {
	Budget  *RetryBudget
	Breaker *BreakerMap
	Metrics *Metrics
	Max     int
}

func NewFailover(budget *RetryBudget, breaker *BreakerMap, metrics *Metrics, maxAttempts int) *Failover {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &Failover{Budget: budget, Breaker: breaker, Metrics: metrics, Max: maxAttempts}
}

func (f *Failover) Do(ctx context.Context, workers []Worker, call CallFunc) (used Worker, err error) {
	if len(workers) == 0 {
		return Worker{}, fmt.Errorf("no workers")
	}

	// Prefer closed breakers; if all open, still try (panic mode).
	ordered := make([]Worker, 0, len(workers))
	var open []Worker
	for _, w := range workers {
		if f.Breaker != nil && f.Breaker.State(w.ID) == StateOpen {
			if err := f.Breaker.Allow(w.ID); err != nil {
				open = append(open, w)
				continue
			}
		}
		ordered = append(ordered, w)
	}
	if len(ordered) == 0 {
		ordered = open
	}

	var last error
	attempts := 0
	for i, w := range ordered {
		if attempts >= f.Max {
			break
		}
		if i > 0 {
			if f.Budget != nil && !f.Budget.Allow() {
				return used, last
			}
		}
		if f.Breaker != nil {
			if err := f.Breaker.Allow(w.ID); err != nil && len(ordered) > 1 {
				continue
			}
		}

		attempts++
		err := call(ctx, w)
		if err == nil {
			if f.Budget != nil {
				f.Budget.OnSuccess()
			}
			if f.Breaker != nil {
				f.Breaker.Success(w.ID)
			}
			return w, nil
		}
		last = err
		if IsExcluded(err) {
			return w, err
		}
		if f.Breaker != nil && !IsExcluded(err) {
			f.Breaker.Failure(w.ID)
		}
		if !Retryable(err) {
			return w, err
		}
		if f.Budget != nil && i > 0 {
			f.Budget.OnFailure()
		} else if f.Budget != nil {
			f.Budget.OnFailure()
		}
		// peek next for metric
		if i+1 < len(ordered) {
			if f.Metrics != nil {
				f.Metrics.IncFailover(w.ID, ordered[i+1].ID, statusReason(err))
			}
		}
	}
	if last == nil {
		last = errors.New("no healthy worker")
	}
	return used, last
}

func statusReason(err error) string {
	if err == nil {
		return "unknown"
	}
	if IsBreakerOpen(err) {
		return "breaker_open"
	}
	return "unavailable"
}
