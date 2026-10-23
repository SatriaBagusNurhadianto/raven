package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteProblemSetsTypeAndRequestID(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/saves/utama", nil)
	req = req.WithContext(WithRequestID(req.Context(), "abc12345"))

	WriteProblem(rec, req, NotFound("slot tidak ada"))

	if got, want := rec.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, mau %d", got, want)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Errorf("Content-Type = %q, mau application/problem+json", ct)
	}

	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("body bukan Problem valid: %v", err)
	}
	if p.Type != TypeNotFound {
		t.Errorf("type = %q, mau %q", p.Type, TypeNotFound)
	}
	if p.Status != 404 || p.Title == "" || p.Detail == "" {
		t.Errorf("problem tidak lengkap: %+v", p)
	}
	if p.RequestID != "abc12345" {
		t.Errorf("request_id = %q, mau abc12345", p.RequestID)
	}
	if p.Instance != "/v1/saves/utama" {
		t.Errorf("instance = %q, mau /v1/saves/utama", p.Instance)
	}
}

func TestWriteProblemRateLimitedSetsRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteProblem(rec, httptest.NewRequest("POST", "/v1/auth/guest", nil), RateLimited("pelan-pelan"))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, mau 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After harus ada untuk 429")
	}
}

func TestWriteProblemUnauthorizedSetsWWWAuthenticate(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteProblem(rec, httptest.NewRequest("GET", "/v1/saves", nil), Unauthorized("token tidak ada"))

	if got := rec.Header().Get("WWW-Authenticate"); !strings.Contains(got, "Bearer") {
		t.Errorf("WWW-Authenticate = %q, mau memuat Bearer", got)
	}
}

func TestInternalDoesNotLeakCauseMessage(t *testing.T) {
	secretCause := errors.New("password pemain: RahasiaSekali")

	rec := httptest.NewRecorder()
	WriteProblem(rec, httptest.NewRequest("GET", "/", nil), Internal(secretCause))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "RahasiaSekali") {
		t.Errorf("pesan error internal bocor ke klien:\n%s", rec.Body.String())
	}
}

func TestValidationCarriesFieldErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteProblem(rec, httptest.NewRequest("POST", "/v1/auth/login", nil),
		Validation("Input tidak lolos validasi.",
			FieldError{Field: "email", Detail: "format email tidak valid"},
			FieldError{Field: "password", Detail: "minimal 8 karakter"},
		))

	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("unmarshal gagal: %v", err)
	}
	if p.Status != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, mau 422", p.Status)
	}
	if len(p.Errors) != 2 {
		t.Fatalf("jumlah field error = %d, mau 2", len(p.Errors))
	}
	if p.Errors[0].Field != "email" {
		t.Errorf("field pertama = %q, mau email", p.Errors[0].Field)
	}
}

func TestProblemFromErrorUnwrapsKnownProblem(t *testing.T) {
	want := Conflict("versi tidak cocok")
	got := ProblemFromError(want)
	if got != want {
		t.Errorf("ProblemFromError tidak mengembalikan Problem asli: %+v", got)
	}

	wrapped := ProblemFromError(errors.New("apa saja"))
	if wrapped.Status != http.StatusInternalServerError {
		t.Errorf("error tak dikenal harus jadi 500, dapat %d", wrapped.Status)
	}
}

func TestProblemErrorInterface(t *testing.T) {
	p := BadRequest("test")
	if !strings.Contains(p.Error(), "400") {
		t.Errorf("Error() = %q, mau memuat status", p.Error())
	}
}
