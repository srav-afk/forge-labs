package scheduler

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/srav-afk/forge-labs/internal/observability"
)

type Metrics struct {
	dispatched *prometheus.CounterVec
}

func NewMetrics(reg *observability.Registry) *Metrics {
	m := &Metrics{
		dispatched: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forge_scheduler_dispatched_total",
			Help: "Requests dispatched to a worker by the online scheduler",
		}, []string{"worker_id", "model"}),
	}
	reg.MustRegister(m.dispatched)
	return m
}

func (m *Metrics) IncDispatched(workerID, model string) {
	if m == nil {
		return
	}
	m.dispatched.WithLabelValues(workerID, model).Inc()
}
