package gateway

import (
	"context"
	"io"
	"strings"

	"google.golang.org/grpc/metadata"

	workerv1 "github.com/srav-afk/forge-labs/gen/worker/v1"
	"github.com/srav-afk/forge-labs/services/provider"
	"github.com/srav-afk/forge-labs/worker/adapters"
)

type ProviderLookup interface {
	BackendByEndpoint(endpoint string) (*provider.OpenAICompat, bool)
	Backend(workerID string) (*provider.OpenAICompat, bool)
}

type chunkStream struct {
	ch  <-chan *workerv1.TokenChunk
	err error
	ctx context.Context
}

func (s *chunkStream) Recv() (*workerv1.TokenChunk, error) {
	c, ok := <-s.ch
	if !ok {
		if s.err != nil {
			return nil, s.err
		}
		return nil, io.EOF
	}
	return c, nil
}

func (s *chunkStream) Header() (metadata.MD, error) { return nil, nil }
func (s *chunkStream) Trailer() metadata.MD         { return nil }
func (s *chunkStream) CloseSend() error             { return nil }
func (s *chunkStream) Context() context.Context     { return s.ctx }
func (s *chunkStream) SendMsg(any) error            { return nil }
func (s *chunkStream) RecvMsg(any) error            { return io.EOF }

func isProviderEndpoint(endpoint string) bool {
	return strings.HasPrefix(endpoint, "provider://")
}

func (h *Handler) openProviderStream(ctx context.Context, worker *SelectedWorker, genReq *workerv1.GenerateRequest) (workerv1.WorkerService_GenerateClient, func(), error) {
	if h.providers == nil {
		return nil, nil, io.ErrUnexpectedEOF
	}
	backend, ok := h.providers.Backend(worker.ID)
	if !ok {
		backend, ok = h.providers.BackendByEndpoint(worker.Endpoint)
	}
	if !ok || backend == nil {
		return nil, nil, io.ErrUnexpectedEOF
	}
	model := genReq.GetModel().GetBaseModel()
	ch := make(chan *workerv1.TokenChunk, 16)
	stream := &chunkStream{ch: ch, ctx: ctx}
	go func() {
		defer close(ch)
		err := backend.Generate(ctx, adapters.GenerateRequest{
			Model:  model,
			Prompt: genReq.GetPrompt(),
		}, func(c adapters.TokenChunk) error {
			msg := &workerv1.TokenChunk{Text: c.Text, Done: c.Done, FinishReason: c.FinishReason}
			if c.Done {
				msg.Usage = &workerv1.Usage{
					PromptTokens:     c.PromptTokens,
					CompletionTokens: c.EvalTokens,
					TotalDurationNs:  c.TotalDurNs,
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case ch <- msg:
				return nil
			}
		})
		stream.err = err
	}()
	return stream, func() {}, nil
}
