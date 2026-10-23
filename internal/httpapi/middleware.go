package httpapi

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// Middleware membungkus handler HTTP.
type Middleware func(http.Handler) http.Handler

// Chain menyusun middleware sehingga yang pertama adalah lapisan terluar.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// MiddlewareOptions mengatur perilaku middleware dasar.
type MiddlewareOptions struct {
	Logger         *slog.Logger
	Metrics        MetricsRecorder
	MaxBodyBytes   int64
	TrustedProxies []string
	CORSOrigins    []string
}

// RequestIDMiddleware memastikan setiap request punya request ID, baik dari
// klien (bila valid) maupun yang dibuat server.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(headerRequestID))
		if !IsValidRequestID(id) {
			id = NewRequestID()
		}
		w.Header().Set(headerRequestID, id)
		next.ServeHTTP(w, r.WithContext(WithRequestID(r.Context(), id)))
	})
}

// ClientIPMiddleware menyisipkan IP klien (menghormati trusted proxy).
func ClientIPMiddleware(trusted []string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ClientIP(r, trusted)
			next.ServeHTTP(w, r.WithContext(WithClientIP(r.Context(), ip)))
		})
	}
}

// RecoverMiddleware menangkap panic di handler dan mengubahnya menjadi 500
// problem+json, bukan proses yang mati (PRD §6 ketersediaan).
func RecoverMiddleware(log *slog.Logger, metrics MetricsRecorder) Middleware {
	if metrics == nil {
		metrics = NopMetrics()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				// http.ErrAbortHandler adalah sinyal sah untuk membatalkan; teruskan.
				if err, ok := rec.(error); ok && err == http.ErrAbortHandler {
					panic(rec)
				}
				metrics.RecordPanic()
				if log != nil {
					log.Error("panic di handler",
						"panic", rec,
						"method", r.Method,
						"path", r.URL.Path,
						"stack", string(debug.Stack()),
					)
				}
				WriteProblem(w, r, Internal(nil))
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// AccessLogMiddleware mencatat setiap request sebagai satu baris JSON.
func AccessLogMiddleware(log *slog.Logger, metrics MetricsRecorder) Middleware {
	if metrics == nil {
		metrics = NopMetrics()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			metrics.IncInFlight(1)
			defer metrics.IncInFlight(-1)

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)

			route := RouteFrom(r.Context())
			dur := time.Since(start)
			metrics.RecordRequest(r.Method, route, statusLabel(sw.Status()), dur)

			if log != nil {
				level := slog.LevelInfo
				switch {
				case sw.Status() >= 500:
					level = slog.LevelError
				case sw.Status() >= 400:
					level = slog.LevelWarn
				}
				log.Log(r.Context(), level, "request http",
					"method", r.Method,
					"route", route,
					"path", r.URL.Path,
					"status", sw.Status(),
					"bytes", sw.written,
					"duration_ms", float64(dur.Microseconds())/1000,
					"client_ip", ClientIPFrom(r.Context()),
				)
			}
		})
	}
}

// BodyLimitMiddleware membatasi ukuran body request (mitigasi DoS, PRD §8).
func BodyLimitMiddleware(maxBytes int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if maxBytes > 0 && r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORSMiddleware menangani cross-origin request untuk SDK berbasis browser.
// Origin diizinkan bila persis cocok dengan daftar, atau bila daftar memuat "*".
func CORSMiddleware(origins []string) Middleware {
	allowed := make(map[string]struct{}, len(origins))
	allowAll := false
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o == "*" {
			allowAll = true
		}
		if o != "" {
			allowed[o] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if _, ok := allowed[origin]; ok || allowAll {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Expose-Headers", "ETag, X-Request-Id, X-RateLimit-Remaining")
				}
			}
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match, Idempotency-Key, X-Request-Id")
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeadersMiddleware menambahkan header keamanan dasar (PRD §8).
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
