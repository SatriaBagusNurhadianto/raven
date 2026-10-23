package obs

import (
	"net/http"
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics menampung collector Raven beserta registry-nya sendiri.
// Registry privat (bukan global) agar test bisa berjalan terisolasi.
type Metrics struct {
	reg *prometheus.Registry

	RequestsTotal    *prometheus.CounterVec
	RequestDuration  *prometheus.HistogramVec
	RequestsInFlight prometheus.Gauge
	RateLimited      *prometheus.CounterVec
	Panics           prometheus.Counter
}

// NewMetrics mendaftarkan seluruh metrik Raven (PRD §6 observability).
func NewMetrics(version string) *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		reg: reg,
		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "raven", Subsystem: "http", Name: "requests_total",
			Help: "Jumlah request HTTP yang ditangani.",
		}, []string{"method", "route", "status"}),
		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "raven", Subsystem: "http", Name: "request_duration_seconds",
			Help:    "Durasi request HTTP.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"method", "route"}),
		RequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "raven", Subsystem: "http", Name: "requests_in_flight",
			Help: "Request yang sedang diproses.",
		}),
		RateLimited: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "raven", Subsystem: "http", Name: "rate_limited_total",
			Help: "Request yang ditolak rate limit.",
		}, []string{"route"}),
		Panics: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "raven", Name: "handler_panics_total",
			Help: "Panic yang tertangkap di handler.",
		}),
	}

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "raven", Name: "build_info", Help: "Info build Raven.",
	}, []string{"version", "go_version"})
	buildInfo.WithLabelValues(version, runtime.Version()).Set(1)

	reg.MustRegister(
		collectors.NewGoCollector(),
		m.RequestsTotal,
		m.RequestDuration,
		m.RequestsInFlight,
		m.RateLimited,
		m.Panics,
		buildInfo,
	)
	return m
}

// Handler mengembalikan handler HTTP untuk endpoint /metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}

// Registry mengembalikan registry privat (dipakai test).
func (m *Metrics) Registry() *prometheus.Registry { return m.reg }
