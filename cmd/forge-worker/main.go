package main

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/srav-afk/forge-labs/internal/config"
	"github.com/srav-afk/forge-labs/internal/observability"
)

func main() {
	cfg := config.Load(map[string]any{
		"metrics.addr": ":9091",
	})

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
