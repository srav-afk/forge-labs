package impl

import (
	"context"
	"encoding/json"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/datatypes"

	registryv1 "github.com/srav-afk/forge-labs/gen/registry/v1"
	"github.com/srav-afk/forge-labs/services/registry"
	"github.com/srav-afk/forge-labs/services/registry/models"
)

type registryService struct {
	registryv1.UnimplementedRegistryServiceServer
	repo registry.WorkerRepository
}

func NewRegistryService(repo registry.WorkerRepository) registry.RegistryService {
	return &registryService{repo: repo}
}

func (s *registryService) Register(ctx context.Context, req *registryv1.RegisterRequest) (*registryv1.RegisterResponse, error) {
	if req.GetWorkerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "worker_id is required")
	}
	if req.GetEndpoint() == "" {
		return nil, status.Error(codes.InvalidArgument, "endpoint is required")
	}

	caps, err := json.Marshal(req.GetCapabilities())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal capabilities: %v", err)
	}

	now := time.Now().UTC()
	w := &models.Worker{
		ID:           req.GetWorkerId(),
		Endpoint:     req.GetEndpoint(),
		RuntimeKind:  req.GetRuntimeKind().String(),
		Capabilities: datatypes.JSON(caps),
		RegisteredAt: now,
	}
	for _, m := range req.GetModels() {
		w.Models = append(w.Models, models.ServableModel{
			BaseModel:  m.GetBaseModel(),
			Adapter:    m.GetAdapter(),
			MaxContext: m.GetMaxContext(),
		})
	}

	if err := s.repo.Upsert(ctx, w); err != nil {
		return nil, status.Errorf(codes.Internal, "upsert worker: %v", err)
	}

	return &registryv1.RegisterResponse{
		WorkerId:     w.ID,
		RegisteredAt: timestamppb.New(now),
	}, nil
}
