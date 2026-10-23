# Raven — The Self-Hosted Game Backend

[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

Raven adalah backend game siap pakai yang dijalankan sendiri oleh developer atau
komunitas game (via Docker, VPS, atau PC sendiri). Menyediakan auth, cloud save,
leaderboard, multi-tenant, dan dashboard admin lewat REST API + SDK Unity/Godot/JS.

Maintainer hanya membangun dan merilis software; **tidak ada infrastruktur hosting
yang dikelola maintainer.**

## Quickstart (< 5 menit)

```bash
cp deploy/.env.example .env
# edit .env (minimal RAVEN_JWT_SECRET di produksi)
docker compose -f deploy/docker-compose.yml up -d
# wizard first-run: buka http://localhost (via Caddy) atau http://localhost:8080
```

Tanpa Docker:

```bash
go run ./cmd/raven doctor      # cek konfigurasi
go run ./cmd/raven migrate up  # terapkan skema
go run ./cmd/raven serve       # jalan di :8080
```

## Endpoint

| Path | Keterangan |
|---|---|
| `GET /healthz` | Liveness |
| `GET /readyz` | Readiness (DB) |
| `GET /metrics` | Metrik Prometheus |
| `GET /v1/meta` | Info build |
| `/v1/auth/*` | Auth (M1 — selesai: guest, register/login, refresh rotation, link, ban) |
| `/v1/saves/*` | Cloud save (M2 — selesai: put/get/list/versions/delete/delta, If-Match) |
| `/v1/leaderboards/*` | Leaderboard (M3 — selesai: submit score, top N, around-me) |
| `/v1/...` | Multi-tenant (M4 — tenant resolution dari API key selesai) |

Format error seragam: RFC 9457 Problem Details (`application/problem+json`).

## Struktur

```
cmd/raven/          entrypoint + subcommand (serve, migrate, doctor, version)
internal/config/    konfigurasi dari env, default aman
internal/obs/       logging JSON + metrik Prometheus
internal/db/        pool pgx + runner migrasi
internal/httpapi/   router, middleware, problem+json
internal/app/       perakitan aplikasi
api/openapi.yaml    spesifikasi OpenAPI 3.1 (single source of truth)
deploy/             Dockerfile, docker-compose, Caddyfile
docs/adr/           keputusan arsitektur
```

## Pengembangan

```bash
go build ./...
go vet ./...
go test ./... -race
```

Lihat `CONTRIBUTING.md` dan `docs/adr/0001-arsitektur-awal.md`.

## Status

M0–M6 sebagian selesai: identity, cloud save, leaderboard, tenant, backoffice, JS SDK, doctor, backup. Lihat progress di vault Obsidian project Raven.

## Lisensi

Apache-2.0 — lihat `LICENSE`.
