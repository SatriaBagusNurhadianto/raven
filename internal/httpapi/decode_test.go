package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type payload struct {
	Name string `json:"name"`
}

func TestDecodeJSONHappyPath(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"dina"}`))
	req.Header.Set("Content-Type", "application/json")

	var p payload
	if prob := DecodeJSON(req, 1024, &p); prob != nil {
		t.Fatalf("DecodeJSON gagal: %v", prob)
	}
	if p.Name != "dina" {
		t.Errorf("Name = %q, mau dina", p.Name)
	}
}

func TestDecodeJSONErrors(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		contentType string
		limit       int64
		wantStatus  int
		wantCode    string
	}{
		{"json rusak", `{"name":`, "application/json", 1024, http.StatusBadRequest, "bad_request"},
		{"body kosong", ``, "application/json", 1024, http.StatusBadRequest, "bad_request"},
		{"field tak dikenal", `{"hacker":1}`, "application/json", 1024, http.StatusBadRequest, "bad_request"},
		{"dua objek", `{"name":"a"}{"name":"b"}`, "application/json", 1024, http.StatusBadRequest, "bad_request"},
		{"terlalu besar", `{"name":"` + strings.Repeat("x", 100) + `"}`, "application/json", 16, http.StatusRequestEntityTooLarge, "payload_too_large"},
		{"media type salah", `{"name":"a"}`, "text/plain", 1024, http.StatusUnsupportedMediaType, "unsupported_media_type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)

			var p payload
			prob := DecodeJSON(req, tc.limit, &p)
			if prob == nil {
				t.Fatal("mau Problem, dapat nil")
			}
			if prob.Status != tc.wantStatus {
				t.Errorf("status = %d, mau %d (%s)", prob.Status, tc.wantStatus, prob.Detail)
			}
			if prob.Code != tc.wantCode {
				t.Errorf("code = %q, mau %q", prob.Code, tc.wantCode)
			}
		})
	}
}

func TestDecodeJSONAllowsProblemJSONContentType(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"a"}`))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	var p payload
	if prob := DecodeJSON(req, 1024, &p); prob != nil {
		t.Fatalf("Content-Type +json harus diterima, dapat: %v", prob)
	}
}

func TestItoa64(t *testing.T) {
	cases := map[int64]string{0: "0", 7: "7", -12: "-12", 1048576: "1048576"}
	for in, want := range cases {
		if got := itoa64(in); got != want {
			t.Errorf("itoa64(%d) = %q, mau %q", in, got, want)
		}
	}
}
