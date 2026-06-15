package main

import (
	"log"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/knadh/koanf/v2"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/dig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gorm.io/gorm"

	registryv1 "github.com/srav-afk/forge-labs/gen/registry/v1"
	"github.com/srav-afk/forge-labs/internal/config"
	"github.com/srav-afk/forge-labs/internal/db"
	"github.com/srav-afk/forge-labs/internal/observability"
	"github.com/srav-afk/forge-labs/services/registry"
	registryimpl "github.com/srav-afk/forge-labs/services/registry/impl"
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
	must(c.Provide(registry.NewWorkerRepository))
	must(c.Provide(registryimpl.NewRegistryService))
	must(c.Provide(observability.NewRegistry))

	must(c.Invoke(run))
}

func run(k *koanf.Koanf, reg *observability.Registry, svc registry.RegistryService) error {
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "forge_controlplane_build_info",
		Help: "Control plane build metadata, always 1.",
	}, []string{"version"})
	reg.MustRegister(buildInfo)
	buildInfo.WithLabelValues(version).Set(1)

	go serveMetrics(k.String("metrics.addr"), reg)
	go serveGRPC(k.String("grpc.addr"), svc)
	return serveHTTP(k.String("http.addr"))
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

func serveHTTP(addr string) error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	return r.Run(addr)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
