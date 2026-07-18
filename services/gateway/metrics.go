package gateway

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/srav-afk/forge-labs/internal/observability"
)

type Metrics struct {
	duration *prometheus.HistogramVec
	ttft     *prometheus.HistogramVec
}

func NewMetrics(reg *observability.Registry) *Metrics {
	return &Metrics{
		duration: observability.NewHistogramVec(reg, prometheus.HistogramOpts{
			Name: "forge_gateway_request_duration_seconds",
			Help: "End-to-end gateway request duration in seconds",
		}, []string{"route", "model", "stream", "status"}),
		ttft: observability.NewHistogramVec(reg, prometheus.HistogramOpts{
			Name: "forge_gateway_ttft_seconds",
			Help: "Time to first token flushed by the gateway",
		}, []string{"route", "model"}),
	}
}

func (m *Metrics) ObserveDuration(route, model string, stream bool, status int, seconds float64) {
	m.duration.WithLabelValues(route, model, strconv.FormatBool(stream), strconv.Itoa(status)).Observe(seconds)
}

func (m *Metrics) ObserveTTFT(route, model string, seconds float64) {
	m.ttft.WithLabelValues(route, model).Observe(seconds)
}
