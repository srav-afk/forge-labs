package workergrpc

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	workerv1 "github.com/srav-afk/forge-labs/gen/worker/v1"
	"github.com/srav-afk/forge-labs/worker/adapters"
)

type Server struct {
	workerv1.UnimplementedWorkerServiceServer
	adapter   adapters.RuntimeAdapter
	keepAlive string
}

func NewServer(adapter adapters.RuntimeAdapter, keepAlive string) *Server {
	return &Server{adapter: adapter, keepAlive: keepAlive}
}

func (s *Server) Generate(req *workerv1.GenerateRequest, stream workerv1.WorkerService_GenerateServer) error {
	tr := otel.Tracer("forge-worker")
	ctx, span := tr.Start(stream.Context(), "worker.Generate")
	defer span.End()

	if req.GetPrompt() == "" {
		span.SetStatus(otelcodes.Error, "prompt required")
		return status.Error(codes.InvalidArgument, "prompt is required")
	}
	model := req.GetModel().GetBaseModel()
	if model == "" {
		span.SetStatus(otelcodes.Error, "model required")
		return status.Error(codes.InvalidArgument, "model.base_model is required")
	}
	adapter := req.GetModel().GetAdapter()
	span.SetAttributes(
		attribute.String("base_model", model),
		attribute.String("adapter", adapter),
		attribute.Int("prompt_chars", len(req.GetPrompt())),
	)

	options := map[string]any{}
	if sp := req.GetSampling(); sp != nil {
		if sp.GetTemperature() > 0 {
			options["temperature"] = sp.GetTemperature()
		}
		if sp.GetTopK() > 0 {
			options["top_k"] = sp.GetTopK()
		}
		if sp.GetTopP() > 0 {
			options["top_p"] = sp.GetTopP()
		}
		if sp.GetMaxTokens() > 0 {
			options["num_predict"] = sp.GetMaxTokens()
		}
	}

	genReq := adapters.GenerateRequest{
		Model:     model,
		Prompt:    req.GetPrompt(),
		Options:   options,
		KeepAlive: s.keepAlive,
	}

	ctx, runtimeSpan := tr.Start(ctx, "runtime.call")
	runtimeSpan.SetAttributes(attribute.String("runtime", "ollama"))
	chunks := 0
	err := s.adapter.Generate(ctx, genReq, func(chunk adapters.TokenChunk) error {
		chunks++
		msg := &workerv1.TokenChunk{
			Text: chunk.Text,
			Done: chunk.Done,
		}
		if chunk.Done {
			msg.FinishReason = chunk.FinishReason
			msg.Usage = &workerv1.Usage{
				PromptTokens:     chunk.PromptTokens,
				CompletionTokens: chunk.EvalTokens,
				TotalDurationNs:  chunk.TotalDurNs,
			}
			runtimeSpan.SetAttributes(
				attribute.Int("prompt_tokens", int(chunk.PromptTokens)),
				attribute.Int("completion_tokens", int(chunk.EvalTokens)),
				attribute.String("finish_reason", chunk.FinishReason),
			)
		}
		return stream.Send(msg)
	})
	runtimeSpan.SetAttributes(attribute.Int("chunks", chunks))
	if err != nil {
		runtimeSpan.RecordError(err)
		runtimeSpan.SetStatus(otelcodes.Error, err.Error())
	}
	runtimeSpan.End()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	return mapError(err)
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "client canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "deadline exceeded")
	}
	var notFound *adapters.ModelNotFoundError
	if errors.As(err, &notFound) {
		return status.Errorf(codes.NotFound, "model %q not found", notFound.Model)
	}
	if st, ok := status.FromError(err); ok {
		return st.Err()
	}
	return status.Errorf(codes.Internal, "generate: %v", err)
}
