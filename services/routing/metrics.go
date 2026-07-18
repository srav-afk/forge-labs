package routing

import (
	"math"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/srav-afk/forge-labs/internal/observability"
)

type Metrics struct {
	age          prometheus.GaugeFunc
	decodeErrors prometheus.Counter
	publishes    prometheus.Counter
}

func NewMetrics(reg *observability.Registry, holder *SnapshotHolder) *Metrics {
	m := &Metrics{
		decodeErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "forge_routing_snapshot_decode_errors_total",
			Help: "Malformed routing snapshot payloads skipped by the subscriber",
		}),
		publishes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "forge_routing_snapshot_publishes_total",
			Help: "Routing snapshots published by the control plane",
		}),
	}
	m.age = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "forge_routing_snapshot_age_seconds",
		Help: "Seconds since the in-memory routing snapshot was built by the control plane",
	}, func() float64 {
		s := holder.Load()
		if s == nil {
			return math.Inf(1)
		}
		return time.Since(s.BuiltAt).Seconds()
	})
	reg.MustRegister(m.age, m.decodeErrors, m.publishes)
	return m
}

func (m *Metrics) IncDecodeError() {
	m.decodeErrors.Inc()
}

func (m *Metrics) IncPublish() {
	m.publishes.Inc()
}
