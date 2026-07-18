package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/knadh/koanf/v2"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	registryv1 "github.com/srav-afk/forge-labs/gen/registry/v1"
	workerv1 "github.com/srav-afk/forge-labs/gen/worker/v1"
	"github.com/srav-afk/forge-labs/internal/config"
	"github.com/srav-afk/forge-labs/internal/observability"
	"github.com/srav-afk/forge-labs/internal/redisx"
	"github.com/srav-afk/forge-labs/worker"
	"github.com/srav-afk/forge-labs/worker/adapters/ollama"
	workergrpc "github.com/srav-afk/forge-labs/worker/grpc"
)

func main() {
	cfg := config.Load(config.WorkerDefaults())

	ollamaAdapter := ollama.New(ollama.Config{
		BaseURL:   cfg.String("ollama.url"),
		KeepAlive: cfg.String("ollama.keep.alive"),
	})

	readyCtx, readyCancel := context.WithTimeout(context.Background(), 3*time.Second)
	ready := ollamaAdapter.Ready(readyCtx)
	readyCancel()
	if os.Getenv("FORGE_WORKER_READY") != "" {
		ready = cfg.Bool("worker.ready")
	}
	if !ready {
		log.Printf("ollama not ready at %s; heartbeats will advertise ready=false", cfg.String("ollama.url"))
	}

	registerWithControlPlane(cfg)

	rdb, err := redisx.NewClient(cfg.String("redis.url"))
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer rdb.Close()

	var adapterName *string
	if a := cfg.String("worker.model.adapter"); a != "" {
		adapterName = &a
	}

	hb := worker.NewHeartbeatWriter(worker.HeartbeatWriterConfig{
		RDB:       rdb,
		ID:        cfg.String("worker.id"),
		BaseModel: cfg.String("worker.model.base"),
		Adapter:   adapterName,
		Runtime:   config.RuntimeLabel(cfg.String("worker.runtime")),
		Addr:      cfg.String("worker.endpoint"),
		TTL:       config.Duration(cfg, "heartbeat.ttl", 6*time.Second),
		Interval:  config.Duration(cfg, "heartbeat.interval", 2*time.Second),
		Ready:     ready,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tr, err := observability.NewTracer(observability.TraceConfig{
		ServiceName:  "forge-worker",
		OTLPEndpoint: cfg.String("otlp.endpoint"),
		SampleRatio:  cfg.Float64("trace.sample.ratio"),
	})
	if err != nil {
		log.Fatalf("tracer: %v", err)
	}
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tr.Shutdown(sctx)
	}()

	go hb.Run(ctx)
	go refreshReady(ctx, ollamaAdapter, hb)

	reg := observability.NewRegistry()
	up := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "forge_worker_up",
		Help: "Worker liveness, always 1 while the process is up.",
	})
	reg.MustRegister(up)
	up.Set(1)

	go serveMetrics(cfg.String("metrics.addr"), reg)
	go serveWorkerGRPC(cfg.String("worker.grpc.addr"), ollamaAdapter, cfg.String("ollama.keep.alive"))

	log.Printf("worker %s gRPC on %s (ollama=%s ready=%v)",
		cfg.String("worker.id"), cfg.String("worker.grpc.addr"), cfg.String("ollama.url"), ready)
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := hb.Delete(shutdownCtx); err != nil {
		log.Printf("heartbeat delete on shutdown: %v", err)
	}
	log.Printf("worker shut down")
}

func refreshReady(ctx context.Context, a *ollama.Adapter, hb *worker.HeartbeatWriter) {
	if os.Getenv("FORGE_WORKER_READY") != "" {
		return
	}
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			probe, cancel := context.WithTimeout(ctx, 2*time.Second)
			hb.SetReady(a.Ready(probe))
			cancel()
		}
	}
}

func serveMetrics(addr string, reg *observability.Registry) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", reg.Handler())
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("metrics server: %v", err)
	}
}

func serveWorkerGRPC(addr string, a *ollama.Adapter, keepAlive string) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("worker grpc listen: %v", err)
	}
	s := grpc.NewServer(observability.GRPCServerOption())
	workerv1.RegisterWorkerServiceServer(s, workergrpc.NewServer(a, keepAlive))
	reflection.Register(s)
	log.Printf("worker gRPC listening on %s", addr)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("worker grpc serve: %v", err)
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
