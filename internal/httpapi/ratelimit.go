package httpapi

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// tokenBucket adalah token bucket per klien.
type tokenBucket struct {
	tokens   float64
	lastSeen time.Time
}

// RateLimiter adalah rate limiter in-memory (default MVP; Redis untuk multi-node
// ditunda ke v1.x sesuai PRD §7.1).
type RateLimiter struct {
	rps   float64
	burst float64

	mu      sync.Mutex
	buckets map[string]*tokenBucket

	now      func() time.Time
	lastGC   time.Time
	gcEvery  time.Duration
	maxIdle  time.Duration
	stopOnce sync.Once
	stop     chan struct{}
}

// NewRateLimiter membuat rate limiter dengan rps dan burst tertentu.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	if rps <= 0 {
		rps = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &RateLimiter{
		rps:     rps,
		burst:   float64(burst),
		buckets: make(map[string]*tokenBucket),
		now:     time.Now,
		gcEvery: time.Minute,
		maxIdle: 10 * time.Minute,
		stop:    make(chan struct{}),
	}
}

// Allow melaporkan apakah satu request dari key boleh lewat, beserta sisa token
// dan waktu tunggu yang disarankan.
func (rl *RateLimiter) Allow(key string) (allowed bool, remaining int, retryAfter time.Duration) {
	now := rl.now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		b = &tokenBucket{tokens: rl.burst, lastSeen: now}
		rl.buckets[key] = b
	} else {
		elapsed := now.Sub(b.lastSeen).Seconds()
		if elapsed > 0 {
			b.tokens += elapsed * rl.rps
			if b.tokens > rl.burst {
				b.tokens = rl.burst
			}
		}
		b.lastSeen = now
	}

	if b.tokens >= 1 {
		b.tokens--
		return true, int(b.tokens), 0
	}

	// Waktu sampai satu token penuh tersedia lagi.
	missing := 1 - b.tokens
	wait := time.Duration(missing/rl.rps*1000) * time.Millisecond
	if wait <= 0 {
		wait = time.Millisecond
	}
	return false, 0, wait
}

// StartGC menjalankan pembersihan bucket idle secara berkala.
func (rl *RateLimiter) StartGC() {
	go func() {
		t := time.NewTicker(rl.gcEvery)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				rl.gc()
			case <-rl.stop:
				return
			}
		}
	}()
}

// Stop menghentikan goroutine pembersih.
func (rl *RateLimiter) Stop() {
	rl.stopOnce.Do(func() { close(rl.stop) })
}

func (rl *RateLimiter) gc() {
	cutoff := rl.now().Add(-rl.maxIdle)
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for k, b := range rl.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(rl.buckets, k)
		}
	}
}

// Size melaporkan jumlah bucket yang dilacak (dipakai test).
func (rl *RateLimiter) Size() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.buckets)
}

// KeyFunc menentukan kunci rate limit untuk sebuah request.
type KeyFunc func(*http.Request) string

// RateLimitByIP membatasi per alamat IP klien.
func RateLimitByIP(r *http.Request) string {
	return "ip:" + ClientIPFrom(r.Context())
}

// RateLimitMiddleware menolak request yang melewati kuota dengan problem+json.
func RateLimitMiddleware(rl *RateLimiter, keyFn KeyFunc, metrics MetricsRecorder) func(http.Handler) http.Handler {
	if keyFn == nil {
		keyFn = RateLimitByIP
	}
	if metrics == nil {
		metrics = NopMetrics()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, remaining, retryAfter := rl.Allow(keyFn(r))
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(int(rl.burst)))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			if !allowed {
				secs := int(retryAfter.Seconds())
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				if metrics != nil {
					metrics.RecordRateLimited(RouteFrom(r.Context()))
				}
				WriteProblem(w, r, RateLimited("Kuota request terlampaui; coba lagi setelah "+strconv.Itoa(secs)+" detik."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
