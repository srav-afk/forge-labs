package health

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/srav-afk/forge-labs/internal/observability"
)

type Metrics struct {
	healthy *prometheus.GaugeVec
	age     *prometheus.GaugeVec
}

func NewMetrics(reg *observability.Registry) *Metrics {
	m := &Metrics{
		healthy: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "forge_workers_healthy",
			Help: "Count of workers currently in the healthy snapshot",
		}, []string{"base_model", "runtime"}),
		age: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "forge_worker_last_heartbeat_age_seconds",
			Help: "Seconds since the worker-supplied heartbeat timestamp",
		}, []string{"worker_id", "base_model", "runtime"}),
	}
	reg.MustRegister(m.healthy, m.age)
	return m
}

func (m *Metrics) Publish(workers []Heartbeat) {
	m.healthy.Reset()
	m.age.Reset()
	nowMs := time.Now().UnixMilli()
	for _, hb := range workers {
		m.healthy.WithLabelValues(hb.BaseModel, hb.Runtime).Inc()
		age := float64(0)
		if hb.TS > 0 {
			age = float64(nowMs-hb.TS) / 1000.0
			if age < 0 {
				age = 0
			}
		}
		m.age.WithLabelValues(hb.ID, hb.BaseModel, hb.Runtime).Set(age)
	}
}
