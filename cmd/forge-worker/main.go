package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/knadh/koanf/v2"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	registryv1 "github.com/srav-afk/forge-labs/gen/registry/v1"
	"github.com/srav-afk/forge-labs/internal/config"
	"github.com/srav-afk/forge-labs/internal/observability"
)

func main() {
	cfg := config.Load(config.WorkerDefaults())

	registerWithControlPlane(cfg)

	reg := observability.NewRegistry()
	up := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "forge_worker_up",
		Help: "Worker liveness, always 1 while the process is up.",
	})
	reg.MustRegister(up)
	up.Set(1)

	mux := http.NewServeMux()
	mux.Handle("/metrics", reg.Handler())
	if err := http.ListenAndServe(cfg.String("metrics.addr"), mux); err != nil {
		log.Fatalf("metrics server: %v", err)
	}
}

func registerWithControlPlane(cfg *koanf.Koanf) {
	conn, err := grpc.NewClient(cfg.String("controlplane.grpc"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("grpc client: %v", err)
	}
	defer conn.Close()

	client := registryv1.NewRegistryServiceClient(conn)
	req := &registryv1.RegisterRequest{
		WorkerId:    cfg.String("worker.id"),
		Endpoint:    cfg.String("worker.endpoint"),
		RuntimeKind: registryv1.RuntimeKind(registryv1.RuntimeKind_value[cfg.String("worker.runtime")]),
		Models: []*registryv1.ServableModel{
			{
				BaseModel:  cfg.String("worker.model.base"),
				MaxContext: uint32(cfg.Int("worker.model.context")),
			},
		},
		Capabilities: cfg.StringMap("worker.capabilities"),
	}

	var lastErr error
	for attempt := 1; attempt <= 15; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resp, err := client.Register(ctx, req)
		cancel()
		if err == nil {
			log.Printf("registered worker %s at %s", resp.GetWorkerId(), resp.GetRegisteredAt().AsTime())
			return
		}
		lastErr = err
		log.Printf("register attempt %d failed: %v", attempt, err)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("register failed after retries: %v", lastErr)
}
