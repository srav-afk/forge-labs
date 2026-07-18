package gateway

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/srav-afk/forge-labs/internal/observability"
)

type Metrics struct {
	duration *prometheus.HistogramVec
	ttft     *prometheus.HistogramVec
	admitted *prometheus.CounterVec
	rejected *prometheus.CounterVec
	inflight *prometheus.GaugeVec
}

func NewMetrics(reg *observability.Registry) *Metrics {
	m := &Metrics{
		duration: observability.NewHistogramVec(reg, prometheus.HistogramOpts{
			Name: "forge_gateway_request_duration_seconds",
			Help: "End-to-end gateway request duration in seconds",
		}, []string{"route", "model", "stream", "status"}),
		ttft: observability.NewHistogramVec(reg, prometheus.HistogramOpts{
			Name: "forge_gateway_ttft_seconds",
			Help: "Time to first token flushed by the gateway",
		}, []string{"route", "model"}),
		admitted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forge_gateway_requests_admitted_total",
			Help: "Requests admitted by the gateway",
		}, []string{"model"}),
		rejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forge_gateway_requests_rejected_total",
			Help: "Requests rejected before work (admission / no capacity)",
		}, []string{"model", "reason"}),
		inflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "forge_worker_inflight",
			Help: "Gateway-tracked in-flight requests per worker",
		}, []string{"worker_id"}),
	}
	reg.MustRegister(m.admitted, m.rejected, m.inflight)
	return m
}

func (m *Metrics) ObserveDuration(route, model string, stream bool, status int, seconds float64) {
	m.duration.WithLabelValues(route, model, strconv.FormatBool(stream), strconv.Itoa(status)).Observe(seconds)
}

func (m *Metrics) ObserveTTFT(route, model string, seconds float64) {
	m.ttft.WithLabelValues(route, model).Observe(seconds)
}

func (m *Metrics) IncAdmitted(model string) {
	if m == nil {
		return
	}
	m.admitted.WithLabelValues(model).Inc()
}

func (m *Metrics) IncRejected(model, reason string) {
	if m == nil {
		return
	}
	m.rejected.WithLabelValues(model, reason).Inc()
}

func (m *Metrics) SetInflight(workerID string, n int) {
	if m == nil {
		return
	}
	m.inflight.WithLabelValues(workerID).Set(float64(n))
}
