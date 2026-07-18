package workergrpc

import (
	"context"
	"errors"

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
	if req.GetPrompt() == "" {
		return status.Error(codes.InvalidArgument, "prompt is required")
	}
	model := req.GetModel().GetBaseModel()
	if model == "" {
		return status.Error(codes.InvalidArgument, "model.base_model is required")
	}

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

	err := s.adapter.Generate(stream.Context(), genReq, func(chunk adapters.TokenChunk) error {
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
		}
		return stream.Send(msg)
	})
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
