# Kebijakan Keamanan

## Melaporkan kerentanan

JANGAN buka issue publik untuk kerentanan keamanan. Kirim email ke maintainer (lihat
profil GitHub) dengan subjek `[SECURITY] Raven`. Kami akan merespons dalam 7 hari kerja.

## Cakupan

- Kerentanan pada kode Raven (auth, save, leaderboard, admin, SDK).
- Konfigurasi default yang tidak aman.

## Di luar cakupan

- Masalah infrastruktur operator (reverse proxy, DB, OS).
- Serangan yang memerlukan akses fisik atau akses admin yang sah.

## Kebijakan disclosure

Setelah patch dirilis, kami menerbitkan advisory GitHub Security dan mencantumkan
CVE bila berlaku. Kami menghargai responsible disclosure.

## Praktik aman operator

- Set `RAVEN_JWT_SECRET` kuat (>=32 karakter) dan unik.
- Jalankan di balik reverse proxy TLS (Caddy bawaan).
- Aktifkan 2FA TOTP untuk admin.
- Jalankan `raven doctor` secara berkala.
- Backup rutin dengan `raven backup`.
