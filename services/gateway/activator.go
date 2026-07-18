package gateway

import (
	"context"
	"fmt"
	"time"
)

type Activator interface {
	NeedsActivation(model string) bool
	Activate(ctx context.Context, model string) error
}

type noopActivator struct{}

func (noopActivator) NeedsActivation(string) bool            { return false }
func (noopActivator) Activate(context.Context, string) error { return nil }

func waitForCapacity(ctx context.Context, selector WorkerSelector, model, prompt string, timeout time.Duration) (*SelectedWorker, error) {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		w, err := selector.SelectWorker(model, prompt)
		if err == nil {
			return w, nil
		}
		last = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	if last == nil {
		last = fmt.Errorf("no capacity after activate")
	}
	return nil, last
}
