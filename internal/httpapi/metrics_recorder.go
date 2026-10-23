package httpapi

import (
	"net/http"
	"strconv"
	"time"
)

// MetricsRecorder adalah bagian metrik yang dipakai lapisan HTTP. Interface
// dipakai supaya httpapi tidak bergantung langsung pada implementasi Prometheus
// dan mudah diuji dengan implementasi kosong.
type MetricsRecorder interface {
	// RecordRequest mencatat satu request: metode, route, status, durasi.
	RecordRequest(method, route, status string, d time.Duration)
	// RecordRateLimited mencatat penolakan rate limit untuk route tertentu.
	RecordRateLimited(route string)
	// RecordPanic mencatat panic yang tertangkap.
	RecordPanic()
	// IncInFlight menambah/mengurangi jumlah request berjalan.
	IncInFlight(delta int)
}

// nopMetrics dipakai bila metrik tidak dikonfigurasi.
type nopMetrics struct{}

func (nopMetrics) RecordRequest(string, string, string, time.Duration) {}
func (nopMetrics) RecordRateLimited(string)                            {}
func (nopMetrics) RecordPanic()                                        {}
func (nopMetrics) IncInFlight(int)                                     {}

// NopMetrics mengembalikan recorder yang tidak melakukan apa pun.
func NopMetrics() MetricsRecorder { return nopMetrics{} }

// statusWriter merekam status code dan ukuran response.
type statusWriter struct {
	http.ResponseWriter
	status  int
	written int64
	wrote   bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.status = status
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.status = http.StatusOK
		w.wrote = true
	}
	n, err := w.ResponseWriter.Write(b)
	w.written += int64(n)
	return n, err
}

// Unwrap memungkinkan http.ResponseController mengakses writer asli.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) Status() int {
	if !w.wrote {
		return http.StatusOK
	}
	return w.status
}

func statusLabel(status int) string { return strconv.Itoa(status) }
