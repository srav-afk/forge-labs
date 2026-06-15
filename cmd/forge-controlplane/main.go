package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/srav-afk/forge-labs/internal/config"
	"github.com/srav-afk/forge-labs/internal/observability"
)

const version = "0.1.0"

func main() {
	cfg := config.Load(map[string]any{
		"http.addr":    ":8080",
		"metrics.addr": ":9090",
	})

	reg := observability.NewRegistry()
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "forge_controlplane_build_info",
		Help: "Control plane build metadata, always 1.",
	}, []string{"version"})
	reg.MustRegister(buildInfo)
	buildInfo.WithLabelValues(version).Set(1)

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", reg.Handler())
		if err := http.ListenAndServe(cfg.String("metrics.addr"), mux); err != nil {
			log.Fatalf("metrics server: %v", err)
		}
	}()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	if err := r.Run(cfg.String("http.addr")); err != nil {
		log.Fatalf("http server: %v", err)
	}
}
