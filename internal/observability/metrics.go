package observability

import "github.com/prometheus/client_golang/prometheus"

func InferenceBuckets() []float64 {
	return []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60}
}

func NewHistogramVec(reg *Registry, opts prometheus.HistogramOpts, labels []string) *prometheus.HistogramVec {
	if opts.Buckets == nil {
		opts.Buckets = InferenceBuckets()
	}
	h := prometheus.NewHistogramVec(opts, labels)
	reg.MustRegister(h)
	return h
}
