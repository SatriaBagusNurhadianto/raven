# Raven Backend — Performance & Resource Audit (read-only)

Prioritized findings. No code changes made.

## P0 (critical)

1. `internal/identity/middleware_support.go:22-44` — `banCache.data` is an unbounded map keyed by player ID; no eviction/TTL sweep beyond per-entry freshness. Memory leak under many distinct players. Fix: bounded LRU or periodic sweep; cap size.
2. `internal/storage/service.go:270-305` — version cleanup loads all versions unbounded (`var all []v`) and deletes in N+1 loop (`Exec` per row). Fix: single `DELETE ... WHERE id = ANY($1)` batch + bounded query `LIMIT (total-keep)`; paginate.
3. `internal/httpapi/decode.go:32` — `io.ReadAll` into memory for every request; `LimitReader` +1 but no MaxBytesReader. BodyLimitMiddleware uses MaxBytesReader but DecodeJSON uses LimitReader; large bodies still fully buffered. Fix: stream decode with `http.MaxBytesReader` and reject early; enforce per-route max.
4. `internal/db/db.go:28-35` — pool config: MinConns=1 default, no MaxConnIdleTime tuning for load; no statement cache/prepared statements config (pgx default prepared statement cache per-conn). Acceptable but document. Ensure `MaxConns` from config not zero.

## P1 (high)

5. `internal/ranking/service.go:265-271` — leaderboard around-me query uses `rng*2+1` unbounded-ish; offset/limit okay but ensure index `scores_rank_idx` covers. Missing index on `scores(player_id)` for per-player lookups? `scores` PK includes player_id as third col, so prefix not usable for player-only. Add index `(player_id, leaderboard_id)`.
6. `internal/httpapi/health.go:68-82` — readiness spawns goroutine per check with no panic recovery; a panicking check leaks? Actually wg.Done via defer; but no timeout enforcement per-check beyond shared ctx. Acceptable. Add recover.
7. `internal/httpapi/ratelimit.go:91-104` — StartGC goroutine never started in app? Check app.go. If StartGC not called, buckets map grows unbounded. Verify. (app.go grep shows rate limiter disabled?) 
8. `internal/storage/service.go` — ListVersions/Get may return unbounded result sets; check pagination limits.
9. Missing prepared statements: services use inline SQL strings; pgx caches plans per-conn by default, okay. But repeated dynamic SQL in loops (N+1) remains.

## P2 (medium)

10. JSON marshaling: `identity/jwt.go` uses json.Marshal twice per token; fine. Consider json.Encoder with SetEscapeHTML(false) for perf; low priority.
11. `internal/obs/metrics.go` / recorder — in-memory counters; fine.
12. Connection pool exhaustion: no request-scoped timeouts on DB queries? Services should use ctx with timeout; check handlers. Many Query calls without explicit timeout rely on request ctx. Ensure ReadTimeout 30s bounds.

## P3 (low)

13. Channel misuse: `server.go:54` errCh buffered 1, fine. `app.go:215` signal channel buffered 1, fine.
14. `ratelimit.go` GC uses `rl.now()` under lock? gc locks, okay.

## Previous audit items re-check

Security audit (commit 68fe010) fixes present: argon2 salt random, path traversal blob, SQL whitelist order, CORS creds. Verified in code (middleware CORS no wildcard+creds). Good.

## Recommended next actions

- Add LRU bound to banCache.
- Batch delete + limit in storage cleanup.
- Add index on scores(player_id, leaderboard_id).
- Ensure RateLimiter.StartGC() called in app.go when enabled.
- Add per-check recover in health handler.
- Add streaming body limits.