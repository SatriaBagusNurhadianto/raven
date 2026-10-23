// Package config memuat konfigurasi Raven dari environment variable.
//
// Prinsip: default aman. Produksi menolak berjalan tanpa TLS yang terkonfirmasi,
// dengan secret default, atau dengan nilai di luar batas yang wajar.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	EnvDevelopment = "development"
	EnvProduction  = "production"

	// DefaultJWTSecret adalah placeholder yang SENGAJA tidak aman supaya
	// produksi menolaknya. Jangan pernah dipakai di instance nyata.
	DefaultJWTSecret = "dev-insecure-change-me"

	MinSaveBytes = 1 << 10  // 1 KiB
	MaxSaveBytes = 16 << 20 // 16 MiB (batas atas FR-S1)
)

// RateLimit mengatur token bucket per klien.
type RateLimit struct {
	Enabled bool
	RPS     float64
	Burst   int
}

// TLS menyatakan bagaimana TLS ditangani di deployment ini.
type TLS struct {
	// TerminatedByProxy: TLS diakhiri reverse proxy (Caddy) di depan Raven.
	TerminatedByProxy bool
	// AllowInsecure: operator secara sadar menerima HTTP polos (mis. jaringan internal).
	AllowInsecure bool
}

// Config adalah konfigurasi runtime Raven.
type Config struct {
	Env           string
	HTTPAddr      string
	PublicBaseURL string

	DatabaseURL     string
	DBMaxConns      int32
	AutoMigrate     bool
	WaitForDB       time.Duration
	ShutdownTimeout time.Duration

	MaxBodyBytes int64
	RateLimit    RateLimit

	LogLevel string

	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	SaveMaxBytes int64
	SaveVersions int
	BlobDir      string

	TLS            TLS
	CORSOrigins    []string
	TrustedProxies []string
}

// Defaults mengembalikan konfigurasi default yang aman untuk pengembangan lokal.
func Defaults() *Config {
	return &Config{
		Env:           EnvDevelopment,
		HTTPAddr:      ":8080",
		PublicBaseURL: "http://localhost:8080",

		DatabaseURL:     "postgres://raven:raven@localhost:5432/raven?sslmode=disable",
		DBMaxConns:      20,
		AutoMigrate:     true,
		WaitForDB:       30 * time.Second,
		ShutdownTimeout: 15 * time.Second,

		MaxBodyBytes: 1 << 20,
		RateLimit:    RateLimit{Enabled: true, RPS: 50, Burst: 100},

		LogLevel: "info",

		JWTSecret:       DefaultJWTSecret,
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,

		SaveMaxBytes: 1 << 20,
		SaveVersions: 5,
		BlobDir:      "./data/blobs",
	}
}

// FromEnv memuat konfigurasi dari environment variable proses.
func FromEnv() (*Config, error) { return Load(os.Getenv) }

// Load memuat konfigurasi memakai fungsi getenv (memudahkan test).
func Load(getenv func(string) string) (*Config, error) {
	c := Defaults()
	var errs []error

	setString(getenv, "RAVEN_ENV", &c.Env)
	setString(getenv, "RAVEN_HTTP_ADDR", &c.HTTPAddr)
	setString(getenv, "RAVEN_PUBLIC_BASE_URL", &c.PublicBaseURL)
	setString(getenv, "RAVEN_DATABASE_URL", &c.DatabaseURL)
	setString(getenv, "RAVEN_LOG_LEVEL", &c.LogLevel)
	setString(getenv, "RAVEN_JWT_SECRET", &c.JWTSecret)
	setString(getenv, "RAVEN_BLOB_DIR", &c.BlobDir)

	appendErr(&errs, setBool(getenv, "RAVEN_AUTO_MIGRATE", &c.AutoMigrate))
	appendErr(&errs, setBool(getenv, "RAVEN_RATE_LIMIT_ENABLED", &c.RateLimit.Enabled))
	appendErr(&errs, setBool(getenv, "RAVEN_TLS_TERMINATED", &c.TLS.TerminatedByProxy))
	appendErr(&errs, setBool(getenv, "RAVEN_ALLOW_INSECURE_HTTP", &c.TLS.AllowInsecure))

	appendErr(&errs, setInt32(getenv, "RAVEN_DB_MAX_CONNS", &c.DBMaxConns))
	appendErr(&errs, setInt64(getenv, "RAVEN_MAX_BODY_BYTES", &c.MaxBodyBytes))
	appendErr(&errs, setInt64(getenv, "RAVEN_SAVE_MAX_BYTES", &c.SaveMaxBytes))
	appendErr(&errs, setInt(getenv, "RAVEN_SAVE_VERSIONS", &c.SaveVersions))
	appendErr(&errs, setInt(getenv, "RAVEN_RATE_LIMIT_BURST", &c.RateLimit.Burst))
	appendErr(&errs, setFloat(getenv, "RAVEN_RATE_LIMIT_RPS", &c.RateLimit.RPS))
	appendErr(&errs, setDuration(getenv, "RAVEN_WAIT_FOR_DB", &c.WaitForDB))
	appendErr(&errs, setDuration(getenv, "RAVEN_SHUTDOWN_TIMEOUT", &c.ShutdownTimeout))
	appendErr(&errs, setDuration(getenv, "RAVEN_ACCESS_TOKEN_TTL", &c.AccessTokenTTL))
	appendErr(&errs, setDuration(getenv, "RAVEN_REFRESH_TOKEN_TTL", &c.RefreshTokenTTL))

	c.CORSOrigins = splitList(getenv("RAVEN_CORS_ORIGINS"))
	c.TrustedProxies = splitList(getenv("RAVEN_TRUSTED_PROXIES"))

	errs = append(errs, c.Validate())
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return c, nil
}

// IsProduction melaporkan apakah instance ini berjalan dengan env produksi.
func (c *Config) IsProduction() bool { return c.Env == EnvProduction }

// Validate memeriksa konsistensi konfigurasi. Semua masalah dikembalikan sekaligus.
func (c *Config) Validate() error {
	var errs []error

	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if c.Env != EnvDevelopment && c.Env != EnvProduction {
		fail("RAVEN_ENV harus %q atau %q, dapat %q", EnvDevelopment, EnvProduction, c.Env)
	}
	if strings.TrimSpace(c.HTTPAddr) == "" {
		fail("RAVEN_HTTP_ADDR wajib diisi")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		fail("RAVEN_DATABASE_URL wajib diisi")
	} else if _, err := url.Parse(c.DatabaseURL); err != nil {
		fail("RAVEN_DATABASE_URL bukan URL yang valid: %v", err)
	}
	if c.PublicBaseURL != "" {
		u, err := url.Parse(c.PublicBaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			fail("RAVEN_PUBLIC_BASE_URL harus URL absolut, dapat %q", c.PublicBaseURL)
		}
	}
	if c.DBMaxConns < 1 {
		fail("RAVEN_DB_MAX_CONNS minimal 1, dapat %d", c.DBMaxConns)
	}
	if c.MaxBodyBytes < 64<<10 {
		fail("RAVEN_MAX_BODY_BYTES minimal 65536, dapat %d", c.MaxBodyBytes)
	}
	if c.SaveMaxBytes < MinSaveBytes || c.SaveMaxBytes > MaxSaveBytes {
		fail("RAVEN_SAVE_MAX_BYTES harus antara %d dan %d, dapat %d", MinSaveBytes, MaxSaveBytes, c.SaveMaxBytes)
	}
	if c.SaveVersions < 1 || c.SaveVersions > 100 {
		fail("RAVEN_SAVE_VERSIONS harus antara 1 dan 100, dapat %d", c.SaveVersions)
	}
	if c.RateLimit.Enabled {
		if c.RateLimit.RPS <= 0 {
			fail("RAVEN_RATE_LIMIT_RPS harus > 0, dapat %v", c.RateLimit.RPS)
		}
		if c.RateLimit.Burst < 1 {
			fail("RAVEN_RATE_LIMIT_BURST minimal 1, dapat %d", c.RateLimit.Burst)
		}
	}
	if c.AccessTokenTTL <= 0 || c.AccessTokenTTL > 24*time.Hour {
		fail("RAVEN_ACCESS_TOKEN_TTL harus > 0 dan <= 24h, dapat %s", c.AccessTokenTTL)
	}
	if c.RefreshTokenTTL < c.AccessTokenTTL {
		fail("RAVEN_REFRESH_TOKEN_TTL (%s) harus lebih panjang dari RAVEN_ACCESS_TOKEN_TTL (%s)", c.RefreshTokenTTL, c.AccessTokenTTL)
	}
	if c.ShutdownTimeout <= 0 {
		fail("RAVEN_SHUTDOWN_TIMEOUT harus > 0")
	}

	// Produksi: default tidak aman harus ditolak (PRD §8 "misconfigurasi operator").
	if c.IsProduction() {
		if !c.TLS.TerminatedByProxy && !c.TLS.AllowInsecure {
			fail("produksi memerlukan TLS: set RAVEN_TLS_TERMINATED=true bila di balik reverse proxy, atau RAVEN_ALLOW_INSECURE_HTTP=true secara sadar")
		}
		if c.JWTSecret == DefaultJWTSecret {
			fail("RAVEN_JWT_SECRET masih nilai default; produksi menolak jalan dengan secret default")
		}
		if len(c.JWTSecret) < 32 {
			fail("RAVEN_JWT_SECRET produksi minimal 32 karakter, dapat %d", len(c.JWTSecret))
		}
	}
	return errors.Join(errs...)
}

// Redacted mengembalikan ringkasan konfigurasi yang aman untuk dicatat ke log.
func (c *Config) Redacted() map[string]any {
	return map[string]any{
		"env":                     c.Env,
		"http_addr":               c.HTTPAddr,
		"public_base_url":         c.PublicBaseURL,
		"database_url":            RedactDSN(c.DatabaseURL),
		"db_max_conns":            c.DBMaxConns,
		"auto_migrate":            c.AutoMigrate,
		"max_body_bytes":          c.MaxBodyBytes,
		"rate_limit_enabled":      c.RateLimit.Enabled,
		"rate_limit_rps":          c.RateLimit.RPS,
		"rate_limit_burst":        c.RateLimit.Burst,
		"log_level":               c.LogLevel,
		"jwt_secret":              redactSecret(c.JWTSecret),
		"access_token_ttl":        c.AccessTokenTTL.String(),
		"refresh_token_ttl":       c.RefreshTokenTTL.String(),
		"save_max_bytes":          c.SaveMaxBytes,
		"save_versions":           c.SaveVersions,
		"blob_dir":                c.BlobDir,
		"tls_terminated_by_proxy": c.TLS.TerminatedByProxy,
		"cors_origins":            c.CORSOrigins,
		"trusted_proxies":         c.TrustedProxies,
	}
}

// RedactDSN menyembunyikan password di dalam DSN Postgres.
func RedactDSN(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[dsn tidak bisa diparse]"
	}
	if u.User == nil {
		return raw
	}
	if _, has := u.User.Password(); has {
		u.User = url.UserPassword(u.User.Username(), "redacted")
	}
	return u.String()
}

func redactSecret(s string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf("[redacted len=%d]", len(s))
}

func appendErr(errs *[]error, err error) {
	if err != nil {
		*errs = append(*errs, err)
	}
}

func setString(getenv func(string) string, key string, dst *string) {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		*dst = v
	}
}

func setBool(getenv func(string) string, key string, dst *bool) error {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("%s harus boolean (true/false), dapat %q", key, v)
	}
	*dst = b
	return nil
}

func setInt(getenv func(string) string, key string, dst *int) error {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("%s harus bilangan bulat, dapat %q", key, v)
	}
	*dst = n
	return nil
}

func setInt32(getenv func(string) string, key string, dst *int32) error {
	var n int
	if err := setInt(getenv, key, &n); err != nil {
		return err
	}
	// Hanya tulis bila env benar-benar diisi; kalau tidak, default tetap utuh.
	if strings.TrimSpace(getenv(key)) != "" {
		*dst = int32(n)
	}
	return nil
}

func setInt64(getenv func(string) string, key string, dst *int64) error {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fmt.Errorf("%s harus bilangan bulat, dapat %q", key, v)
	}
	*dst = n
	return nil
}

func setFloat(getenv func(string) string, key string, dst *float64) error {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fmt.Errorf("%s harus angka, dapat %q", key, v)
	}
	*dst = f
	return nil
}

func setDuration(getenv func(string) string, key string, dst *time.Duration) error {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("%s harus durasi Go (mis. 15m, 30s), dapat %q", key, v)
	}
	*dst = d
	return nil
}

func splitList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
