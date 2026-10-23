package config

import (
	"strings"
	"testing"
	"time"
)

// envFrom membuat fungsi getenv palsu dari map.
func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaultsInDevelopment(t *testing.T) {
	c, err := Load(envFrom(nil))
	if err != nil {
		t.Fatalf("Load default gagal: %v", err)
	}
	if c.Env != EnvDevelopment {
		t.Errorf("Env = %q, mau %q", c.Env, EnvDevelopment)
	}
	if c.AccessTokenTTL != 15*time.Minute {
		t.Errorf("AccessTokenTTL = %s, mau 15m", c.AccessTokenTTL)
	}
	if c.RateLimit.RPS != 50 || c.RateLimit.Burst != 100 {
		t.Errorf("RateLimit = %+v, mau {50 100}", c.RateLimit)
	}
	if !c.AutoMigrate {
		t.Error("AutoMigrate default harus true")
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"env tidak dikenal", map[string]string{"RAVEN_ENV": "staging"}, "RAVEN_ENV"},
		{"durasi rusak", map[string]string{"RAVEN_ACCESS_TOKEN_TTL": "lima belas menit"}, "RAVEN_ACCESS_TOKEN_TTL"},
		{"bool rusak", map[string]string{"RAVEN_AUTO_MIGRATE": "yes please"}, "RAVEN_AUTO_MIGRATE"},
		{"save terlalu besar", map[string]string{"RAVEN_SAVE_MAX_BYTES": "33554432"}, "RAVEN_SAVE_MAX_BYTES"},
		{"save terlalu kecil", map[string]string{"RAVEN_SAVE_MAX_BYTES": "16"}, "RAVEN_SAVE_MAX_BYTES"},
		{"refresh lebih pendek dari access", map[string]string{
			"RAVEN_ACCESS_TOKEN_TTL":  "1h",
			"RAVEN_REFRESH_TOKEN_TTL": "5m",
		}, "RAVEN_REFRESH_TOKEN_TTL"},
		{"rate limit rps nol", map[string]string{"RAVEN_RATE_LIMIT_RPS": "0"}, "RAVEN_RATE_LIMIT_RPS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(envFrom(tc.env))
			if err == nil {
				t.Fatalf("mau error memuat %v, dapat nil", tc.env)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q tidak menyebut %q", err, tc.want)
			}
		})
	}
}

func TestProductionRequiresTLS(t *testing.T) {
	strong := strings.Repeat("k", 40)

	base := map[string]string{
		"RAVEN_ENV":        EnvProduction,
		"RAVEN_JWT_SECRET": strong,
	}

	if _, err := Load(envFrom(base)); err == nil {
		t.Fatal("produksi tanpa konfirmasi TLS harus gagal")
	}

	withProxy := map[string]string{"RAVEN_ENV": EnvProduction, "RAVEN_JWT_SECRET": strong, "RAVEN_TLS_TERMINATED": "true"}
	if _, err := Load(envFrom(withProxy)); err != nil {
		t.Fatalf("produksi dengan TLS proxy harus lolos, dapat: %v", err)
	}

	insecureOK := map[string]string{"RAVEN_ENV": EnvProduction, "RAVEN_JWT_SECRET": strong, "RAVEN_ALLOW_INSECURE_HTTP": "true"}
	if _, err := Load(envFrom(insecureOK)); err != nil {
		t.Fatalf("RAVEN_ALLOW_INSECURE_HTTP harus membuat produksi lolos, dapat: %v", err)
	}
}

func TestProductionRejectsWeakJWTSecret(t *testing.T) {
	_, err := Load(envFrom(map[string]string{
		"RAVEN_ENV":            EnvProduction,
		"RAVEN_TLS_TERMINATED": "true",
	}))
	if err == nil {
		t.Fatal("produksi dengan JWT secret default harus gagal")
	}
	if !strings.Contains(err.Error(), "default") {
		t.Errorf("error harus menyebut secret default, dapat %q", err)
	}

	_, err = Load(envFrom(map[string]string{
		"RAVEN_ENV":            EnvProduction,
		"RAVEN_TLS_TERMINATED": "true",
		"RAVEN_JWT_SECRET":     "pendek",
	}))
	if err == nil || !strings.Contains(err.Error(), "32 karakter") {
		t.Errorf("mau error panjang minimum 32 karakter, dapat %v", err)
	}
}

func TestRedactedHidesSecrets(t *testing.T) {
	c, err := Load(envFrom(map[string]string{
		"RAVEN_DATABASE_URL": "postgres://raven:sup3rs3cret@db:5432/raven",
		"RAVEN_JWT_SECRET":   "jangan-sampai-bocor",
	}))
	if err != nil {
		t.Fatalf("Load gagal: %v", err)
	}
	got := c.Redacted()
	dsn, _ := got["database_url"].(string)
	if strings.Contains(dsn, "sup3rs3cret") {
		t.Errorf("password DSN bocor: %s", dsn)
	}
	if !strings.Contains(dsn, "redacted") {
		t.Errorf("password DSN harus diganti penanda, dapat %s", dsn)
	}
	secret, _ := got["jwt_secret"].(string)
	if strings.Contains(secret, "jangan-sampai-bocor") {
		t.Errorf("JWT secret bocor: %s", secret)
	}
	if !strings.HasPrefix(secret, "[redacted") {
		t.Errorf("JWT secret harus ditandai redacted, dapat %s", secret)
	}
}

func TestRedactDSNWithoutPassword(t *testing.T) {
	in := "postgres://raven@localhost/raven"
	if got := RedactDSN(in); got != in {
		t.Errorf("RedactDSN(%q) = %q, mau tidak berubah", in, got)
	}
}

func TestSplitList(t *testing.T) {
	got := splitList(" https://a.example , https://b.example ,, ")
	if len(got) != 2 || got[0] != "https://a.example" || got[1] != "https://b.example" {
		t.Errorf("splitList = %v", got)
	}
	if splitList("   ") != nil {
		t.Error("splitList string kosong harus nil")
	}
}
