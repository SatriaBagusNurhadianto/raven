# ADR 0001: Arsitektur Awal Raven

- Status: Accepted
- Tanggal: 2026-10-22

## Konteks

Raven adalah backend game self-hosted: auth, cloud save, leaderboard, multi-tenant,
dashboard admin, SDK Unity/Godot/JS. Target: `docker compose up` <5 menit, idle
API <100 MB RAM, image <60 MB, p95 <100 ms CRUD (PRD §6).

## Keputusan

1. **Bahasa: Go.** Single binary, cross-compile mudah, footprint kecil. (PRD §7.1)
2. **Router: stdlib `net/http` ServeMux** (method patterns Go 1.22+), bukan chi.
   Alasan: nol dependency, cukup untuk REST, membantu target footprint. Deviasi dari
   kebiasaan umum dicatat di sini agar tidak dianggap kelalaian.
3. **Database: PostgreSQL** sebagai satu-satunya database v1, via pgx/v5. (PRD §7.1)
4. **Migrasi: runner embedded sendiri** (embed.FS + tabel `schema_migrations`),
   bukan goose/atlas. Alasan: mengurangi dependency; migrasi tetap reversible (blok Down).
5. **Dependensi minimal:** pgx/v5, golang.org/x/crypto, prometheus/client_golang.
   Sisanya stdlib (JWT ditulis manual ~80 LOC, slog, net/http).
6. **Blob store:** disk lokal default; S3-compatible opsional di v1.x.
7. **Identitas pemain MVP: terisolasi per game** (`players.game_id` NOT NULL).
   Opsi lintas-game disiapkan sebagai flag instance (`instance_settings.identity_mode`),
   belum diimplementasi.

## Konsekuensi

- Setup developer lebih sederhana (go build tanpa tooling tambahan).
- Penambahan dependency baru harus direview (lihat CONTRIBUTING).
- Kompatibilitas API mundur dalam satu major version (PRD §6).
