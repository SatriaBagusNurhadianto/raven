package obs

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsHandlerExposesBuildInfo(t *testing.T) {
	m := NewMetrics("test-v1")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)

	m.Handler().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("kode = %d, mau 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "raven_build_info") {
		t.Errorf("metrik raven_build_info tidak ada di output:\n%s", body)
	}
	if !strings.Contains(body, `version="test-v1"`) {
		t.Errorf("label version tidak terisi:\n%s", body)
	}
	if !strings.Contains(body, "go_goroutines") {
		t.Error("metrik runtime Go tidak terekspos")
	}
}

func TestMetricsCountersRecord(t *testing.T) {
	m := NewMetrics("test")
	m.RequestsTotal.WithLabelValues("GET", "/healthz", "200").Inc()

	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("Gather gagal: %v", err)
	}
	found := false
	for _, f := range families {
		if f.GetName() == "raven_http_requests_total" {
			found = true
			if len(f.GetMetric()) != 1 {
				t.Errorf("jumlah seri = %d, mau 1", len(f.GetMetric()))
			}
			if got := f.GetMetric()[0].GetCounter().GetValue(); got != 1 {
				t.Errorf("nilai counter = %v, mau 1", got)
			}
		}
	}
	if !found {
		t.Error("raven_http_requests_total tidak terdaftar")
	}
}
