package httpapi

import (
	"log/slog"
	"net/http"
	"sort"
	"strings"
)

// Route mendeskripsikan satu endpoint yang terdaftar.
type Route struct {
	Method  string
	Pattern string
	Name    string
	Handler http.Handler
}

// RouterOptions mengatur pembangunan router.
type RouterOptions struct {
	Logger         *slog.Logger
	Metrics        MetricsRecorder
	MaxBodyBytes   int64
	TrustedProxies []string
	CORSOrigins    []string
	RateLimiter    *RateLimiter
}

// Router mendaftarkan endpoint dan menyusun rantai middleware.
type Router struct {
	opts   RouterOptions
	mux    *http.ServeMux
	routes []Route
}

// NewRouter membuat router baru beserta middleware default yang aman.
func NewRouter(opts RouterOptions) *Router {
	if opts.Metrics == nil {
		opts.Metrics = NopMetrics()
	}
	rt := &Router{opts: opts, mux: http.NewServeMux()}
	rt.mux.Handle("/", rt.notFoundHandler())
	return rt
}

// Handle mendaftarkan handler pada method + pola ServeMux Go 1.22
// (mis. "GET /v1/saves/{slot}"). Nama dipakai sebagai label metrik.
func (rt *Router) Handle(method, pattern, name string, h http.Handler) *Router {
	if h == nil {
		panic("httpapi: handler nil untuk " + method + " " + pattern)
	}
	route := Route{Method: method, Pattern: pattern, Name: name, Handler: h}
	rt.routes = append(rt.routes, route)

	inner := h
	if rt.opts.RateLimiter != nil {
		inner = RateLimitMiddleware(rt.opts.RateLimiter, RateLimitByIP, rt.opts.Metrics)(inner)
	}
	// Nama route ditulis ke context supaya log & metrik bisa melabelinya.
	withName := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next := WithRoute(r.Context(), name)
		inner.ServeHTTP(w, r.WithContext(next))
	})
	rt.mux.Handle(method+" "+pattern, withName)
	return rt
}

// HandleFunc adalah versi ringkas Handle untuk http.HandlerFunc.
func (rt *Router) HandleFunc(method, pattern, name string, h http.HandlerFunc) *Router {
	return rt.Handle(method, pattern, name, h)
}

// Get mendaftarkan endpoint GET.
func (rt *Router) Get(pattern, name string, h http.HandlerFunc) *Router {
	return rt.HandleFunc(http.MethodGet, pattern, name, h)
}

// Post mendaftarkan endpoint POST.
func (rt *Router) Post(pattern, name string, h http.HandlerFunc) *Router {
	return rt.HandleFunc(http.MethodPost, pattern, name, h)
}

// Put mendaftarkan endpoint PUT.
func (rt *Router) Put(pattern, name string, h http.HandlerFunc) *Router {
	return rt.HandleFunc(http.MethodPut, pattern, name, h)
}

// Delete mendaftarkan endpoint DELETE.
func (rt *Router) Delete(pattern, name string, h http.HandlerFunc) *Router {
	return rt.HandleFunc(http.MethodDelete, pattern, name, h)
}

// Routes mengembalikan salinan daftar route, terurut, untuk dokumentasi.
func (rt *Router) Routes() []Route {
	out := make([]Route, 0, len(rt.routes))
	for _, r := range rt.routes {
		if r.Pattern == "/" {
			continue // handler catch-all, bukan endpoint nyata
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pattern == out[j].Pattern {
			return out[i].Method < out[j].Method
		}
		return out[i].Pattern < out[j].Pattern
	})
	return out
}

// Handler menyusun rantai middleware lengkap di sekitar mux.
func (rt *Router) Handler() http.Handler {
	return Chain(rt.mux,
		RequestIDMiddleware,                        // terluar: semua log punya ID
		ClientIPMiddleware(rt.opts.TrustedProxies), // sebelum rate limit & log
		SecurityHeadersMiddleware,
		CORSMiddleware(rt.opts.CORSOrigins),
		RecoverMiddleware(rt.opts.Logger, rt.opts.Metrics),
		AccessLogMiddleware(rt.opts.Logger, rt.opts.Metrics),
		BodyLimitMiddleware(rt.opts.MaxBodyBytes),
	)
}

// notFoundHandler mengembalikan 404 dalam format problem+json.
func (rt *Router) notFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// OPTIONS ditangani middleware CORS; sisanya 404 seragam.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		WriteProblem(w, r, NotFound("Endpoint "+r.Method+" "+sanitizePath(r.URL.Path)+" tidak ditemukan."))
	})
}

// sanitizePath membatasi panjang path di dalam pesan error.
func sanitizePath(p string) string {
	if len(p) > 200 {
		return p[:200] + "..."
	}
	if strings.ContainsAny(p, "\r\n") {
		return "[path tidak valid]"
	}
	return p
}
