package obs

import (
	"strconv"
	"time"
)

// Recorder mengimplementasikan httpapi.MetricsRecorder memakai *Metrics.
// httpapi tidak mengimpor obs dan sebaliknya; kecocokan terjadi lewat struktur
// method (interface Go), sehingga tidak ada dependensi melingkar.
type Recorder struct{ M *Metrics }

// NewRecorder membungkus *Metrics sebagai recorder HTTP.
func NewRecorder(m *Metrics) *Recorder { return &Recorder{M: m} }

// RecordRequest mencatat satu request selesai.
func (r *Recorder) RecordRequest(method, route, status string, d time.Duration) {
	if r == nil || r.M == nil {
		return
	}
	if route == "" {
		route = "unmatched"
	}
	r.M.RequestsTotal.WithLabelValues(method, route, status).Inc()
	r.M.RequestDuration.WithLabelValues(method, route).Observe(d.Seconds())
}

// RecordRateLimited mencatat penolakan rate limit.
func (r *Recorder) RecordRateLimited(route string) {
	if r == nil || r.M == nil {
		return
	}
	if route == "" {
		route = "unmatched"
	}
	r.M.RateLimited.WithLabelValues(route).Inc()
}

// RecordPanic mencatat panic yang tertangkap.
func (r *Recorder) RecordPanic() {
	if r == nil || r.M == nil {
		return
	}
	r.M.Panics.Inc()
}

// IncInFlight menambah/mengurangi gauge request berjalan.
func (r *Recorder) IncInFlight(delta int) {
	if r == nil || r.M == nil {
		return
	}
	r.M.RequestsInFlight.Add(float64(delta))
}

// StatusLabel mengubah status code menjadi string label.
func StatusLabel(status int) string { return strconv.Itoa(status) }
