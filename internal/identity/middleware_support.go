package identity

import (
	"context"
	"sync"
	"time"
)

// banCache menyimpan status ban per player dengan TTL agar DB tidak diquery
// setiap request, sambil menjamin deteksi ban dalam <=60s (FR-A6).
type banCache struct {
	mu   sync.Mutex
	data map[string]banEntry
	ttl  time.Duration
}

type banEntry struct {
	banned bool
	at     time.Time
}

func newBanCache() *banCache { return &banCache{data: map[string]banEntry{}, ttl: 30 * time.Second} }

func (c *banCache) check(ctx context.Context, playerID string, fetch func(context.Context, string) (bool, error)) (bool, error) {
	c.mu.Lock()
	e, ok := c.data[playerID]
	fresh := ok && time.Since(e.at) < c.ttl
	c.mu.Unlock()
	if fresh {
		return e.banned, nil
	}
	banned, err := fetch(ctx, playerID)
	if err != nil {
		return false, err
	}
	c.mu.Lock()
	c.data[playerID] = banEntry{banned: banned, at: time.Now()}
	c.mu.Unlock()
	return banned, nil
}

// Invalidate menghapus cache (dipakai setelah ban/unban).
func (c *banCache) Invalidate(playerID string) {
	c.mu.Lock()
	delete(c.data, playerID)
	c.mu.Unlock()
}
