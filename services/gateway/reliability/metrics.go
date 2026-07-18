package reliability

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/srav-afk/forge-labs/internal/observability"
)

type Metrics struct {
	breakerState *prometheus.GaugeVec
	failovers    *prometheus.CounterVec
}

func NewMetrics(reg *observability.Registry) *Metrics {
	m := &Metrics{
		breakerState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "forge_worker_circuit_breaker_state",
			Help: "Circuit breaker state per worker (0=closed, 1=half-open, 2=open)",
		}, []string{"worker_id"}),
		failovers: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forge_gateway_failover_total",
			Help: "Transparent failovers before first token",
		}, []string{"from_worker", "to_worker", "reason"}),
	}
	reg.MustRegister(m.breakerState, m.failovers)
	return m
}

func (m *Metrics) SetBreakerState(workerID string, state State) {
	if m == nil {
		return
	}
	m.breakerState.WithLabelValues(workerID).Set(float64(state))
}

func (m *Metrics) IncFailover(from, to, reason string) {
	if m == nil {
		return
	}
	m.failovers.WithLabelValues(from, to, reason).Inc()
}
