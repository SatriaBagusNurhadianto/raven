// Package httpapi menyediakan router, middleware, dan helper HTTP untuk Raven.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

// Problem adalah representasi error sesuai RFC 9457 Problem Details (PRD §7.4).
type Problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
	Code     string `json:"code,omitempty"`
	// RequestID memudahkan korelasi dengan log.
	RequestID string `json:"request_id,omitempty"`
	// Errors memuat detail validasi per field.
	Errors []FieldError `json:"errors,omitempty"`
}

// Error membuat Problem memenuhi interface error.
func (p *Problem) Error() string {
	return fmt.Sprintf("%s (%d): %s", p.Title, p.Status, p.Detail)
}

// FieldError menjelaskan satu kegagalan validasi input.
type FieldError struct {
	Field  string `json:"field"`
	Detail string `json:"detail"`
}

const (
	problemBase = "https://raven.dev/problems/"

	TypeBadRequest     = problemBase + "bad-request"
	TypeValidation     = problemBase + "validation-failed"
	TypeUnauthorized   = problemBase + "unauthorized"
	TypeForbidden      = problemBase + "forbidden"
	TypeNotFound       = problemBase + "not-found"
	TypeConflict       = problemBase + "conflict"
	TypeRateLimited    = problemBase + "rate-limited"
	TypePayloadTooBig  = problemBase + "payload-too-large"
	TypeUnsupported    = problemBase + "unsupported-media-type"
	TypeBanned         = problemBase + "player-banned"
	TypeInternal       = problemBase + "internal-error"
	TypeServiceUnavail = problemBase + "service-unavailable"
)

var statusTitles = map[int]string{
	http.StatusBadRequest:            "Permintaan tidak valid",
	http.StatusUnauthorized:          "Tidak terautentikasi",
	http.StatusForbidden:             "Akses ditolak",
	http.StatusNotFound:              "Tidak ditemukan",
	http.StatusConflict:              "Konflik",
	http.StatusTooManyRequests:       "Terlalu banyak permintaan",
	http.StatusRequestEntityTooLarge: "Payload terlalu besar",
	http.StatusUnsupportedMediaType:  "Tipe konten tidak didukung",
	http.StatusUnprocessableEntity:   "Tidak dapat diproses",
	http.StatusInternalServerError:   "Kesalahan internal",
	http.StatusServiceUnavailable:    "Layanan tidak tersedia",
}

// NewProblem membuat Problem dengan judul standar untuk status tersebut.
func NewProblem(status int, problemType, detail, code string) *Problem {
	title, ok := statusTitles[status]
	if !ok {
		title = http.StatusText(status)
	}
	return &Problem{Type: problemType, Title: title, Status: status, Detail: detail, Code: code}
}

// BadRequest membuat Problem 400.
func BadRequest(detail string) *Problem {
	return NewProblem(400, TypeBadRequest, detail, "bad_request")
}

// Validation membuat Problem 422 dengan detail per field.
func Validation(detail string, fields ...FieldError) *Problem {
	p := NewProblem(http.StatusUnprocessableEntity, TypeValidation, detail, "validation_failed")
	p.Errors = fields
	return p
}

// Unauthorized membuat Problem 401.
func Unauthorized(detail string) *Problem {
	return NewProblem(401, TypeUnauthorized, detail, "unauthorized")
}

// Forbidden membuat Problem 403.
func Forbidden(detail string) *Problem { return NewProblem(403, TypeForbidden, detail, "forbidden") }

// NotFound membuat Problem 404.
func NotFound(detail string) *Problem { return NewProblem(404, TypeNotFound, detail, "not_found") }

// Conflict membuat Problem 409.
func Conflict(detail string) *Problem { return NewProblem(409, TypeConflict, detail, "conflict") }

// RateLimited membuat Problem 429.
func RateLimited(detail string) *Problem {
	return NewProblem(http.StatusTooManyRequests, TypeRateLimited, detail, "rate_limited")
}

// PayloadTooLarge membuat Problem 413.
func PayloadTooLarge(detail string) *Problem {
	return NewProblem(http.StatusRequestEntityTooLarge, TypePayloadTooBig, detail, "payload_too_large")
}

// UnsupportedMediaType membuat Problem 415.
func UnsupportedMediaType(detail string) *Problem {
	return NewProblem(http.StatusUnsupportedMediaType, TypeUnsupported, detail, "unsupported_media_type")
}

// Banned membuat Problem 403 untuk pemain yang diblokir (FR-A6).
func Banned(detail string) *Problem { return NewProblem(403, TypeBanned, detail, "player_banned") }

// ServiceUnavailable membuat Problem 503.
func ServiceUnavailable(detail string) *Problem {
	return NewProblem(http.StatusServiceUnavailable, TypeServiceUnavail, detail, "service_unavailable")
}

// Internal membuat Problem 500. Detail teknis TIDAK ikut dikirim ke klien;
// penyebab aslinya dicatat ke log oleh pemanggil.
func Internal(cause error) *Problem {
	_ = cause
	return NewProblem(500, TypeInternal, "Terjadi kesalahan internal.", "internal_error")
}

// WriteProblem menulis Problem sebagai response JSON beserta request ID.
func WriteProblem(w http.ResponseWriter, r *http.Request, p *Problem) {
	if p == nil {
		p = Internal(nil)
	}
	if p.Status == 0 {
		p.Status = http.StatusInternalServerError
	}
	if r != nil {
		if id := RequestIDFrom(r.Context()); id != "" && p.RequestID == "" {
			p.RequestID = id
		}
		if p.Instance == "" {
			p.Instance = r.URL.Path
		}
	}
	for k, v := range ProblemHeaders(p) {
		w.Header().Set(k, v)
	}
	WriteJSONStatus(w, p.Status, p)
}

// ProblemHeaders mengembalikan header yang relevan untuk sebuah Problem.
func ProblemHeaders(p *Problem) map[string]string {
	h := map[string]string{"Content-Type": "application/problem+json; charset=utf-8"}
	if p.Status == http.StatusTooManyRequests {
		h["Retry-After"] = "1"
	}
	if p.Status == http.StatusUnauthorized {
		h["WWW-Authenticate"] = `Bearer realm="raven"`
	}
	return h
}

// ProblemFromError mengubah error biasa menjadi Problem. Error yang sudah
// berupa *Problem dikembalikan apa adanya.
func ProblemFromError(err error) *Problem {
	if err == nil {
		return nil
	}
	var p *Problem
	if errors.As(err, &p) {
		return p
	}
	return Internal(err)
}

// LogProblem mencatat Problem ke log dengan level sesuai statusnya.
func LogProblem(log *slog.Logger, r *http.Request, p *Problem, cause error) {
	if log == nil {
		return
	}
	attrs := []any{
		"status", p.Status,
		"code", p.Code,
		"path", r.URL.Path,
		"method", r.Method,
	}
	if cause != nil {
		attrs = append(attrs, "cause", cause.Error())
	}
	if p.Status >= 500 {
		log.Error(p.Title, attrs...)
		return
	}
	log.Debug(p.Title, attrs...)
}

// WriteJSON menulis payload JSON dengan status 200.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	WriteJSONStatus(w, status, payload)
}

// WriteJSONStatus menulis payload JSON dengan status tertentu.
func WriteJSONStatus(w http.ResponseWriter, status int, payload any) {
	// Hormati Content-Type yang sudah diset pemanggil (problem+json, dsb).
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// Header sudah terkirim; tidak ada yang bisa dilakukan selain berhenti.
		return
	}
}

// ErrorResponse adalah bentuk payload error sederhana untuk endpoint internal
// (mis. /readyz) yang tidak memakai problem+json.
type ErrorResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}
