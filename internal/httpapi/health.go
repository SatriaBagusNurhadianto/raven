package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Check adalah satu pemeriksaan kesiapan (mis. koneksi database).
type Check struct {
	Name string
	Fn   func(context.Context) error
}

// Health menyediakan handler liveness dan readiness (PRD §6).
type Health struct {
	Version   string
	StartedAt time.Time
	Timeout   time.Duration
	Checks    []Check
}

// livenessResponse adalah payload /healthz.
type livenessResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

// readinessResponse adalah payload /readyz.
type readinessResponse struct {
	Status  string            `json:"status"`
	Version string            `json:"version"`
	Checks  map[string]string `json:"checks"`
}

// LivenessHandler selalu 200 selama proses hidup.
func (h *Health) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		started := h.StartedAt
		if started.IsZero() {
			started = time.Now()
		}
		WriteJSON(w, http.StatusOK, livenessResponse{
			Status:  "ok",
			Version: h.Version,
			Uptime:  time.Since(started).Round(time.Second).String(),
		})
	}
}

// ReadinessHandler menjalankan semua Check; 503 bila ada yang gagal.
func (h *Health) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		timeout := h.Timeout
		if timeout <= 0 {
			timeout = 2 * time.Second
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		results := make(map[string]string, len(h.Checks))
		var mu sync.Mutex
		var wg sync.WaitGroup
		ready := true

		for _, c := range h.Checks {
			wg.Add(1)
			go func(c Check) {
				defer wg.Done()
				defer func() {
					if rec := recover(); rec != nil {
						mu.Lock()
						results[c.Name] = "gagal: panic"
						ready = false
						mu.Unlock()
					}
				}()
				status := "ok"
				if err := c.Fn(ctx); err != nil {
					status = "gagal: " + err.Error()
					mu.Lock()
					ready = false
					mu.Unlock()
				}
				mu.Lock()
				results[c.Name] = status
				mu.Unlock()
			}(c)
		}
		wg.Wait()

		resp := readinessResponse{Status: "ok", Version: h.Version, Checks: results}
		if !ready {
			resp.Status = "tidak siap"
			WriteJSON(w, http.StatusServiceUnavailable, resp)
			return
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}
