package gateway

import (
	"context"
	"errors"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	workerv1 "github.com/srav-afk/forge-labs/gen/worker/v1"
	"github.com/srav-afk/forge-labs/services/gateway/reliability"
)

func (h *Handler) withFailover(
	ctx context.Context,
	workers []SelectedWorker,
	fn func(ctx context.Context, w *SelectedWorker) error,
) (*SelectedWorker, error) {
	if len(workers) == 0 {
		return nil, errors.New("no workers")
	}
	if h.failover == nil {
		w := workers[0]
		return &w, fn(ctx, &w)
	}
	cands := make([]reliability.Worker, 0, len(workers))
	byID := map[string]SelectedWorker{}
	for _, w := range workers {
		cands = append(cands, reliability.Worker{ID: w.ID, Endpoint: w.Endpoint})
		byID[w.ID] = w
	}
	used, err := h.failover.Do(ctx, cands, func(ctx context.Context, w reliability.Worker) error {
		sw := byID[w.ID]
		return fn(ctx, &sw)
	})
	if err != nil {
		return nil, err
	}
	sw := byID[used.ID]
	return &sw, nil
}

func (h *Handler) collectWithFailover(ctx context.Context, workers []SelectedWorker, genReq *workerv1.GenerateRequest) (string, *usage, string, *SelectedWorker, error) {
	var text string
	var u *usage
	var finish string
	used, err := h.withFailover(ctx, workers, func(ctx context.Context, w *SelectedWorker) error {
		t, uu, f, err := h.collect(ctx, w, genReq)
		if err != nil {
			return err
		}
		text, u, finish = t, uu, f
		return nil
	})
	return text, u, finish, used, err
}

func (h *Handler) openGenerate(ctx context.Context, worker *SelectedWorker, genReq *workerv1.GenerateRequest) (workerv1.WorkerService_GenerateClient, func(), error) {
	if worker != nil && isProviderEndpoint(worker.Endpoint) {
		return h.openProviderStream(ctx, worker, genReq)
	}
	client, closer, err := h.dial(ctx, worker.Endpoint)
	if err != nil {
		return nil, nil, status.Errorf(codes.Unavailable, "dial worker: %v", err)
	}
	stream, err := client.Generate(ctx, genReq)
	if err != nil {
		closer()
		return nil, nil, err
	}
	return stream, closer, nil
}

// streamFirstToken tries workers until first token chunk or non-retryable error.
// Returns stream + closer for the successful worker, and the first chunk if already received.
func (h *Handler) streamFirstToken(ctx context.Context, workers []SelectedWorker, genReq *workerv1.GenerateRequest) (
	stream workerv1.WorkerService_GenerateClient,
	closer func(),
	worker *SelectedWorker,
	first *workerv1.TokenChunk,
	err error,
) {
	var last error
	for i, w := range workers {
		if i > 0 && h.failover != nil && h.failover.Budget != nil && !h.failover.Budget.Allow() {
			return nil, nil, nil, nil, last
		}
		if h.failover != nil && h.failover.Breaker != nil {
			if e := h.failover.Breaker.Allow(w.ID); e != nil && len(workers) > 1 {
				continue
			}
		}
		s, c, e := h.openGenerate(ctx, &w, genReq)
		if e != nil {
			last = e
			h.recordFail(&w, e)
			if !reliability.Retryable(e) {
				return nil, nil, nil, nil, e
			}
			if h.failover != nil && h.failover.Budget != nil {
				h.failover.Budget.OnFailure()
			}
			if i+1 < len(workers) && h.failover != nil && h.failover.Metrics != nil {
				h.failover.Metrics.IncFailover(w.ID, workers[i+1].ID, "unavailable")
			}
			continue
		}
		chunk, e := s.Recv()
		if e != nil {
			c()
			last = e
			h.recordFail(&w, e)
			if errors.Is(e, io.EOF) {
				// empty generation — treat as success path with no first chunk
				ww := w
				if h.failover != nil {
					if h.failover.Budget != nil {
						h.failover.Budget.OnSuccess()
					}
					if h.failover.Breaker != nil {
						h.failover.Breaker.Success(w.ID)
					}
				}
				return s, c, &ww, nil, nil
			}
			if !reliability.Retryable(e) {
				return nil, nil, nil, nil, e
			}
			if h.failover != nil && h.failover.Budget != nil {
				h.failover.Budget.OnFailure()
			}
			if i+1 < len(workers) && h.failover != nil && h.failover.Metrics != nil {
				h.failover.Metrics.IncFailover(w.ID, workers[i+1].ID, "unavailable")
			}
			continue
		}
		ww := w
		if h.failover != nil {
			if h.failover.Budget != nil {
				h.failover.Budget.OnSuccess()
			}
			if h.failover.Breaker != nil {
				h.failover.Breaker.Success(w.ID)
			}
		}
		return s, c, &ww, chunk, nil
	}
	if last == nil {
		last = errors.New("no healthy worker")
	}
	return nil, nil, nil, nil, last
}

func (h *Handler) recordFail(w *SelectedWorker, err error) {
	if h.failover == nil || reliability.IsExcluded(err) {
		return
	}
	if h.failover.Breaker != nil {
		h.failover.Breaker.Failure(w.ID)
	}
}

func (h *Handler) rankWorkers(model, prompt string) ([]SelectedWorker, error) {
	if sw, ok := h.selector.(interface {
		SelectWorkers(string, string, int) ([]SelectedWorker, error)
	}); ok {
		return sw.SelectWorkers(model, prompt, h.maxAttempts)
	}
	w, err := h.selector.SelectWorker(model, prompt)
	if err != nil {
		return nil, err
	}
	return []SelectedWorker{*w}, nil
}
