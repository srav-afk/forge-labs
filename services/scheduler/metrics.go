package scheduler

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/srav-afk/forge-labs/internal/observability"
)

type Metrics struct {
	dispatched  *prometheus.CounterVec
	filtered    *prometheus.CounterVec
	ewma        *prometheus.GaugeVec
	score       *prometheus.GaugeVec
	affinityHit prometheus.Counter
	decisions   prometheus.Counter
}

func NewMetrics(reg *observability.Registry) *Metrics {
	m := &Metrics{
		dispatched: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forge_scheduler_dispatched_total",
			Help: "Requests dispatched to a worker by the online scheduler",
		}, []string{"worker_id", "model"}),
		filtered: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forge_scheduler_filtered_total",
			Help: "Candidates excluded by health filter",
		}, []string{"reason"}),
		ewma: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "forge_scheduler_worker_ewma_latency_ms",
			Help: "Per-worker EWMA of observed completion latency in milliseconds",
		}, []string{"worker_id"}),
		score: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "forge_scheduler_score",
			Help: "Composite scheduler score of the last pick per worker",
		}, []string{"worker_id"}),
		affinityHit: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "forge_affinity_hits_total",
			Help: "Routing decisions that matched HRW-preferred worker (routing consistency, not true cache hit)",
		}),
		decisions: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "forge_routing_decisions_total",
			Help: "Total routing decisions considered for affinity hit-rate",
		}),
	}
	reg.MustRegister(m.dispatched, m.filtered, m.ewma, m.score, m.affinityHit, m.decisions)
	return m
}

func (m *Metrics) IncDispatched(workerID, model string) {
	if m == nil {
		return
	}
	m.dispatched.WithLabelValues(workerID, model).Inc()
}

func (m *Metrics) IncFiltered(reason string) {
	if m == nil {
		return
	}
	m.filtered.WithLabelValues(reason).Inc()
}

func (m *Metrics) SetEWMA(workerID string, ms float64) {
	if m == nil {
		return
	}
	m.ewma.WithLabelValues(workerID).Set(ms)
}

func (m *Metrics) SetScore(workerID string, score float64) {
	if m == nil {
		return
	}
	m.score.WithLabelValues(workerID).Set(score)
}

func (m *Metrics) IncAffinityHit() {
	if m == nil {
		return
	}
	m.affinityHit.Inc()
}

func (m *Metrics) IncRoutingDecision() {
	if m == nil {
		return
	}
	m.decisions.Inc()
}
