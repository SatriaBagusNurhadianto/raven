# Implementation Plan: Raven M0 — Fondasi

## Overview
Fondasi project: struktur, config, logging, metrik, router, error format, health/readiness, migrasi, packaging, dokumentasi.

## Architecture Decisions
- Go + stdlib router (ADR 0001).
- Postgres via pgx/v5; migrasi embedded.
- Minimal dependency.

## Task List
### Phase 1: Foundation
- [x] T-001 Modul Go + struktur paket
- [x] T-002 Config dari env
- [x] T-003 Logger JSON + request ID
- [x] T-004 Router + middleware
- [x] T-005 Format error RFC 9457
- [x] T-006 Endpoint health/ready/metrics
- [x] T-007 Runner migrasi + 0001_init.sql
- [x] T-008 Packaging Docker/compose
- [x] T-009 OpenAPI 3.1 skeleton
- [x] T-010 Dokumen repo
- [x] T-011 CI

### Checkpoint M0
- [x] go build, go vet, go test bersih
- [x] Server merespons /healthz, /readyz, /metrics
- [x] Migrasi up/down di-review manual (eksekusi nyata menunggu Postgres tersedia)

## Risks and Mitigations
| Risk | Impact | Mitigation |
|---|---|---|
| SQL migrasi belum dieksekusi nyata (tidak ada Postgres lokal) | Medium | Validasi sintaks dilakukan manual; integration test siap dijalankan saat Postgres tersedia |
| Deviasi router stdlib vs chi | Low | ADR 0001 mencatat alasan |
