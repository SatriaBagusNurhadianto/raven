package obs

import (
	"testing"
	"time"
)

func TestRecorderImplementsInterfaceShape(t *testing.T) {
	// Kontrak struktural yang dipakai httpapi.MetricsRecorder. Bila tanda tangan
	// method berubah, test ini gagal saat kompilasi.
	var _ interface {
		RecordRequest(method, route, status string, d time.Duration)
		RecordRateLimited(route string)
		RecordPanic()
		IncInFlight(delta int)
	} = NewRecorder(NewMetrics("test"))
}

func TestRecorderRecords(t *testing.T) {
	m := NewMetrics("test")
	r := NewRecorder(m)

	r.RecordRequest("GET", "/healthz", StatusLabel(200), 5*time.Millisecond)
	r.RecordRateLimited("/v1/auth/guest")
	r.RecordPanic()
	r.IncInFlight(1)
	r.IncInFlight(-1)

	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("Gather gagal: %v", err)
	}
	want := map[string]bool{
		"raven_http_requests_total":           false,
		"raven_http_request_duration_seconds": false,
		"raven_http_rate_limited_total":       false,
		"raven_handler_panics_total":          false,
		"raven_http_requests_in_flight":       false,
	}
	for _, f := range families {
		if _, ok := want[f.GetName()]; ok {
			want[f.GetName()] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("metrik %s tidak terdaftar", name)
		}
	}
}

func TestRecorderNilSafe(t *testing.T) {
	var r *Recorder
	r.RecordRequest("GET", "/", "200", time.Millisecond)
	r.RecordRateLimited("/")
	r.RecordPanic()
	r.IncInFlight(1)
}
