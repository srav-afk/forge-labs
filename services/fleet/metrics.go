package fleet

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/srav-afk/forge-labs/internal/observability"
)

type Metrics struct {
	scaleEvents *prometheus.CounterVec
	desired     *prometheus.GaugeVec
	ready       *prometheus.GaugeVec
}

func NewMetrics(reg *observability.Registry) *Metrics {
	m := &Metrics{
		scaleEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forge_fleet_scale_events_total",
			Help: "Fleet scale up/down events",
		}, []string{"model", "direction"}),
		desired: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "forge_fleet_desired_replicas",
			Help: "Desired replica count per model",
		}, []string{"model"}),
		ready: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "forge_fleet_ready_replicas",
			Help: "Ready replica count per model",
		}, []string{"model"}),
	}
	if reg != nil {
		reg.MustRegister(m.scaleEvents, m.desired, m.ready)
	}
	return m
}

func (m *Metrics) IncScale(model, direction string) {
	if m != nil {
		m.scaleEvents.WithLabelValues(model, direction).Inc()
	}
}

func (m *Metrics) SetDesired(model string, n int) {
	if m != nil {
		m.desired.WithLabelValues(model).Set(float64(n))
	}
}

func (m *Metrics) SetReady(model string, n int) {
	if m != nil {
		m.ready.WithLabelValues(model).Set(float64(n))
	}
}
