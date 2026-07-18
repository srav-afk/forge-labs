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
	"github.com/srav-afk/forge-labs/services/catalog"
	"github.com/srav-afk/forge-labs/services/fleet"
	"github.com/srav-afk/forge-labs/services/gateway"
	"github.com/srav-afk/forge-labs/services/gateway/reliability"
	"github.com/srav-afk/forge-labs/services/health"
	"github.com/srav-afk/forge-labs/services/planner"
	"github.com/srav-afk/forge-labs/services/provider"
	"github.com/srav-afk/forge-labs/services/registry"
	registryimpl "github.com/srav-afk/forge-labs/services/registry/impl"
	"github.com/srav-afk/forge-labs/services/routing"
	"github.com/srav-afk/forge-labs/services/scheduler"
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
	must(c.Provide(catalog.NewRepository))
	must(c.Provide(catalog.NewSnapshotHolder))
	must(c.Provide(func(repo catalog.Repository, holder *catalog.SnapshotHolder, rdb *redis.Client) *catalog.Service {
		return catalog.NewService(repo, holder, rdb)
	}))
	must(c.Provide(observability.NewRegistry))
	must(c.Provide(health.NewMetrics))
	must(c.Provide(func(rdb *redis.Client, m *health.Metrics, k *koanf.Koanf) *health.Service {
		return health.NewService(rdb, m, config.Duration(k, "heartbeat.reconcile", 3*time.Second))
	}))
	must(c.Provide(routing.NewSnapshotHolder))
	must(c.Provide(routing.NewPolicyHolder))
	must(c.Provide(routing.NewInflightTracker))
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
	must(c.Provide(planner.NewObjectiveStore))
	must(c.Provide(func(db *gorm.DB, rdb *redis.Client) *planner.PolicyStore {
		return planner.NewPolicyStore(db, rdb)
	}))
	must(c.Provide(func(
		obj *planner.ObjectiveStore,
		pol *planner.PolicyStore,
		holder *routing.SnapshotHolder,
		ph *routing.PolicyHolder,
	) *planner.Service {
		return planner.NewService(obj, pol, holder, ph)
	}))
	must(c.Provide(scheduler.NewMetrics))
	must(c.Provide(func(sm *scheduler.Metrics, k *koanf.Koanf) *scheduler.LatencyStore {
		return scheduler.NewLatencyStore(config.Duration(k, "scheduler.ewma.tau", 10*time.Second), sm)
	}))
	must(c.Provide(func(sm *scheduler.Metrics, ph *routing.PolicyHolder, k *koanf.Koanf) *scheduler.Chain {
		return scheduler.NewConfiguredChain(scheduler.ChainConfig{
			WeightLoad:     k.Float64("scheduler.weight.load"),
			WeightLatency:  k.Float64("scheduler.weight.latency"),
			WeightAffinity: k.Float64("scheduler.weight.affinity"),
			WeightCost:     k.Float64("scheduler.weight.cost"),
			WeightPolicy:   0.2,
			LatencyRefMs:   k.Float64("scheduler.latency.ref.ms"),
			AdmissionLimit: k.Int("admission.per.worker.limit"),
			AffinityWindow: k.Int("affinity.prefix.window"),
			AffinityBlock:  k.Int("affinity.block.bytes"),
			Metrics:        sm,
			Policy:         scheduler.NewPolicyScorer(ph),
		})
	}))
	must(c.Provide(gateway.NewMetrics))
	must(c.Provide(func(
		holder *routing.SnapshotHolder,
		catalogHolder *catalog.SnapshotHolder,
		inflight *routing.InflightTracker,
		latency *scheduler.LatencyStore,
		chain *scheduler.Chain,
		sm *scheduler.Metrics,
		k *koanf.Koanf,
	) gateway.WorkerSelector {
		return gateway.NewSnapshotSelector(holder, catalogHolder, inflight, latency, chain, sm, k.Int("admission.per.worker.limit"))
	}))
	must(c.Provide(func(reg *observability.Registry) *reliability.Metrics {
		return reliability.NewMetrics(reg)
	}))
	must(c.Provide(func(rm *reliability.Metrics, k *koanf.Koanf) *reliability.BreakerMap {
		return reliability.NewBreakerMap(reliability.BreakerConfig{
			MinRequests:  uint32(k.Int("reliability.breaker.min.requests")),
			FailureRatio: k.Float64("reliability.breaker.failure.ratio"),
			Timeout:      config.Duration(k, "reliability.breaker.timeout", 5*time.Second),
			MaxHalfOpen:  2,
		}, func(workerID string, state reliability.State) {
			rm.SetBreakerState(workerID, state)
		})
	}))
	must(c.Provide(func(k *koanf.Koanf) *reliability.RetryBudget {
		return reliability.NewRetryBudget(
			k.Float64("reliability.retry.budget.tokens"),
			k.Float64("reliability.retry.budget.ratio"),
		)
	}))
	must(c.Provide(func(
		budget *reliability.RetryBudget,
		breakers *reliability.BreakerMap,
		rm *reliability.Metrics,
		k *koanf.Koanf,
	) *reliability.Failover {
		return reliability.NewFailover(budget, breakers, rm, k.Int("reliability.max.attempts"))
	}))
	must(c.Provide(func(
		selector gateway.WorkerSelector,
		inflight *routing.InflightTracker,
		latency *scheduler.LatencyStore,
		gm *gateway.Metrics,
		fo *reliability.Failover,
		k *koanf.Koanf,
	) *gateway.Handler {
		return gateway.NewHandler(
			selector,
			inflight,
			latency,
			gm,
			fo,
			gateway.HandlerConfig{
				AdmissionLimit: k.Int("admission.per.worker.limit"),
				RetryAfterSec:  k.Int("admission.retry.after.seconds"),
				MaxAttempts:    k.Int("reliability.max.attempts"),
			},
		)
	}))
	must(c.Provide(fleet.NewPolicyCache))
	must(c.Provide(func(k *koanf.Koanf) fleet.Provisioner {
		if k.Bool("fleet.runpod.enabled") {
			return fleet.NewRunPodProvisioner(true)
		}
		return fleet.NewLocalProcess()
	}))
	must(c.Provide(func(reg *observability.Registry) *fleet.Metrics {
		return fleet.NewMetrics(reg)
	}))
	must(c.Provide(func(
		pol *fleet.PolicyCache,
		prov fleet.Provisioner,
		holder *routing.SnapshotHolder,
		rdb *redis.Client,
		fm *fleet.Metrics,
	) *fleet.Manager {
		return fleet.NewManager(pol, prov, holder, rdb, fm)
	}))
	must(c.Provide(func(mgr *fleet.Manager) gateway.Activator {
		return mgr
	}))
	must(c.Provide(provider.NewRegistry))

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
	catalogSvc *catalog.Service,
	plannerSvc *planner.Service,
	policyHolder *routing.PolicyHolder,
	fleetMgr *fleet.Manager,
	activator gateway.Activator,
	providers *provider.Registry,
	gw *gateway.Handler,
) error {
	defer rdb.Close()

	tr, err := observability.NewTracer(observability.TraceConfig{
		ServiceName:  "forge-controlplane",
		OTLPEndpoint: k.String("otlp.endpoint"),
		SampleRatio:  k.Float64("trace.sample.ratio"),
	})
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tr.Shutdown(ctx)
	}()

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "forge_controlplane_build_info",
		Help: "Control plane build metadata, always 1.",
	}, []string{"version"})
	reg.MustRegister(buildInfo)
	buildInfo.WithLabelValues(version).Set(1)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	healthSvc.Start(ctx)
	catalogSvc.Start(ctx)
	plannerSvc.Start(ctx)
	fleetMgr.Start(ctx)
	providers.Start(ctx)
	pub.SetVirtualSource(providers)
	gw.SetActivator(activator)
	gw.SetProviders(providers)
	go routing.RunSubscriber(ctx, rdb, holder, rm)
	go routing.RunPolicySubscriber(ctx, rdb, policyHolder)
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
	s := grpc.NewServer(observability.GRPCServerOption())
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
	r.Use(observability.GinMiddleware("forge-controlplane"))
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
