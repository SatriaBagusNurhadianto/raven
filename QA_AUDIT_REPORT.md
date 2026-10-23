# Raven Go Backend — QA Audit Report (read-only)

## Scope
Packages under `internal/`: identity, ranking, storage, backoffice, tenant, httpapi, db, config, obs.

## Test inventory
- Unit: config, db/migrate, httpapi/decode, httpapi/problem, obs/*.
- Integration: backoffice, db/migrate, identity, ranking, storage, tenant (require DB).

## Critical gaps (P0)

1. **identity.Service — no unit tests** (`internal/identity/service.go`)
   - Guest/Register/Login/Link/Refresh/Logout/LogoutAll/Ban/Unban/IsBanned all untested at unit level; only one coarse integration flow.
   - Missing negative tests: invalid email (Register:47, Link:144), short password (Register:51, Link:148), duplicate email (Register:55-60, Link:152-157), wrong password (Login:118), banned (Login:126, Guest:41), deleted (Login:130), reuse detection (Refresh:174-178), expired refresh (Refresh:179-183), idempotent Logout (Logout:237-246).

2. **ranking.Service.SubmitScore — submissions count bug** (`internal/ranking/service.go:105-168`)
   - Local `submissions` variable computed but result returns stale `submissions` (prior+1) while DB increments atomically; on conflict DB does `scores.submissions + 1`, so returned value can mismatch under races. Also `sum` mode uses `bestScore` (prior best) not prior sum — likely incorrect accumulation.
   - Missing tests for ScoreMode best/latest/sum, sort asc/desc, min/max bounds, period reset keys (daily/weekly/monthly), GetAroundMe, tenant isolation.

3. **storage.Service — tenant isolation & concurrency** (`internal/storage/service.go`)
   - No tests verifying cross-player/cross-game access denial at service layer (only integration happy paths).
   - `pruneVersions` (line ~275) called after commit; failure silently ignored; no test.
   - `IfMatch=0` ambiguity: 0 means "no condition" but version 0 never valid; document/test.
   - Blob write happens before DB commit; orphan blobs on failure not cleaned.

4. **backoffice.Service — RBAC & audit** (`internal/backoffice/service.go`)
   - Only one integration test. HasRole (line 109) untested for role hierarchy. Audit (line 101) untested. CreateAdmin/Login untested for invalid creds, duplicate.

5. **tenant.Service.ResolveByAPIKey** (`internal/tenant/service.go:32`) — only happy path; missing not-found, revoked key, caching/concurrency.

## Flaky test risks
- Integration tests depend on live DB; no transaction rollback isolation visible → cross-test contamination.
- `now time.Time` injected in ranking but tests may use wall clock; period boundaries flaky near midnight.
- Storage pruneVersions goroutine? none, but async prune after commit can race with ListVersions.

## Missing negative tests (httpapi)
- decode errors covered; middleware/ratelimit/auth middleware untested.

## Recommendations (prioritized)
P0: Add unit tests for identity service (mock store) covering auth flows, refresh rotation/reuse, ban.
P0: Fix ranking sum mode + submissions return; add unit/integration tests for all score modes and periods.
P1: Add storage concurrency + tenant isolation tests; verify IfMatch conflicts.
P1: Backoffice RBAC matrix + audit tests.
P2: httpapi middleware/ratelimit tests; migrate down/edge cases.

## No code changes made.
