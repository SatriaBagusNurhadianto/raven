# Berkontribusi ke Raven

Terima kasih atas minat Anda. Raven adalah proyek open-source self-hosted game backend.

## Alur

1. Buka issue untuk diskusi fitur/bug sebelum PR besar.
2. Fork, buat branch dari `main`.
3. Pastikan `go vet ./...` dan `go test ./...` lulus.
4. PR merujuk issue dan bagian PRD yang relevan.

## Aturan kode

- Ikuti `gofmt` dan `go vet`.
- Tambah test untuk perilaku baru (target cakupan inti >80%).
- Secret tidak pernah masuk log atau commit.
- Query terparameterisasi; validasi input di boundary.
- Perubahan skema DB butuh migrasi baru (up + down), bukan edit migrasi lama.
- Perubahan API yang merusak kompatibilitas butuh RFC dan bump major version.

## Commit

Gunakan conventional commits: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`.

## Lisensi

Kontribusi Anda dilisensikan di bawah Apache-2.0.
