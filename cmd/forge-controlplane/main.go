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

	"github.com/gin-gonic/gin"
	"github.com/knadh/koanf/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"go.uber.org/dig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gorm.io/gorm"

	registryv1 "github.com/srav-afk/forge-labs/gen/registry/v1"
	"github.com/srav-afk/forge-labs/internal/config"
	"github.com/srav-afk/forge-labs/internal/db"
	"github.com/srav-afk/forge-labs/internal/observability"
	"github.com/srav-afk/forge-labs/internal/redisx"
	"github.com/srav-afk/forge-labs/services/gateway"
	"github.com/srav-afk/forge-labs/services/health"
	"github.com/srav-afk/forge-labs/services/registry"
	registryimpl "github.com/srav-afk/forge-labs/services/registry/impl"
	"github.com/srav-afk/forge-labs/services/routing"
)

const version = "0.1.0"

func main() {
	c := dig.New()

	must(c.Provide(func() *koanf.Koanf {
		return config.Load(config.ControlPlaneDefaults())
	}))
	must(c.Provide(func(k *koanf.Koanf) (*gorm.DB, error) {
		return db.NewGorm(k.String("db.url"))
	}))
	must(c.Provide(func(k *koanf.Koanf) (*redis.Client, error) {
		return redisx.NewClient(k.String("redis.url"))
	}))
	must(c.Provide(registry.NewWorkerRepository))
	must(c.Provide(registryimpl.NewRegistryService))
	must(c.Provide(observability.NewRegistry))
	must(c.Provide(health.NewMetrics))
	must(c.Provide(func(rdb *redis.Client, m *health.Metrics, k *koanf.Koanf) *health.Service {
		return health.NewService(rdb, m, config.Duration(k, "heartbeat.reconcile", 3*time.Second))
	}))
	must(c.Provide(routing.NewSnapshotHolder))
	must(c.Provide(func(reg *observability.Registry, holder *routing.SnapshotHolder) *routing.Metrics {
		return routing.NewMetrics(reg, holder)
	}))
	must(c.Provide(func(
		rdb *redis.Client,
		repo registry.WorkerRepository,
		healthSvc *health.Service,
		holder *routing.SnapshotHolder,
		rm *routing.Metrics,
		k *koanf.Koanf,
	) *routing.Publisher {
		return routing.NewPublisher(
			rdb,
			repo,
			healthSvc,
			holder,
			rm,
			config.Duration(k, "routing.snapshot.interval", 500*time.Millisecond),
		)
	}))
	must(c.Provide(gateway.NewMetrics))
	must(c.Provide(func(holder *routing.SnapshotHolder) gateway.WorkerSelector {
		return gateway.NewSnapshotSelector(holder)
	}))
	must(c.Provide(gateway.NewHandler))

	must(c.Invoke(run))
}

func run(
	k *koanf.Koanf,
	reg *observability.Registry,
	svc registry.RegistryService,
	healthSvc *health.Service,
	rdb *redis.Client,
	holder *routing.SnapshotHolder,
	rm *routing.Metrics,
	pub *routing.Publisher,
	gw *gateway.Handler,
) error {
	defer rdb.Close()

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "forge_controlplane_build_info",
		Help: "Control plane build metadata, always 1.",
	}, []string{"version"})
	reg.MustRegister(buildInfo)
	buildInfo.WithLabelValues(version).Set(1)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	healthSvc.Start(ctx)
	go routing.RunSubscriber(ctx, rdb, holder, rm)
	pub.Start(ctx)

	go serveMetrics(k.String("metrics.addr"), reg)
	go serveGRPC(k.String("grpc.addr"), svc)

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTP(k.String("http.addr"), gw)
	}()

	select {
	case <-ctx.Done():
		log.Printf("controlplane shutting down")
		return nil
	case err := <-errCh:
		return err
	}
}

func serveMetrics(addr string, reg *observability.Registry) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", reg.Handler())
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("metrics server: %v", err)
	}
}

func serveGRPC(addr string, svc registry.RegistryService) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("grpc listen: %v", err)
	}
	s := grpc.NewServer()
	registryv1.RegisterRegistryServiceServer(s, svc)
	reflection.Register(s)
	log.Printf("registry gRPC listening on %s", addr)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("grpc serve: %v", err)
	}
}

func serveHTTP(addr string, gw *gateway.Handler) error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	gw.Register(r)
	log.Printf("gateway http listening on %s", addr)
	return r.Run(addr)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
