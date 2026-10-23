package obs

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":  slog.LevelDebug,
		"INFO":   slog.LevelInfo,
		" warn ": slog.LevelWarn,
		"error":  slog.LevelError,
		"ngawur": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, mau %v", in, got, want)
		}
	}
}

func TestLoggerRedactsSecrets(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "debug")

	log.Info("login pemain",
		"email", "dina@example.com",
		"password", "RahasiaSekali123",
		"refresh_token", "rt-abc-123",
		slog.Group("auth", slog.String("api_key", "key-rahasia")),
	)

	out := buf.String()
	for _, leak := range []string{"RahasiaSekali123", "rt-abc-123", "key-rahasia"} {
		if strings.Contains(out, leak) {
			t.Fatalf("rahasia %q bocor ke log:\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "dina@example.com") {
		t.Errorf("field non-rahasia hilang dari log:\n%s", out)
	}

	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &entry); err != nil {
		t.Fatalf("log bukan JSON valid: %v\n%s", err, out)
	}
	if entry["password"] != "[redacted]" {
		t.Errorf("password = %v, mau [redacted]", entry["password"])
	}
	auth, ok := entry["auth"].(map[string]any)
	if !ok {
		t.Fatalf("grup auth tidak ada: %v", entry["auth"])
	}
	if auth["api_key"] != "[redacted]" {
		t.Errorf("api_key = %v, mau [redacted]", auth["api_key"])
	}
}

func TestLoggerWithAttrsRedacts(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "info").With("jwt_secret", "bocor-ini")

	log.Info("mulai")

	if strings.Contains(buf.String(), "bocor-ini") {
		t.Fatalf("rahasia dari With() bocor:\n%s", buf.String())
	}
}

func TestIsSecretKey(t *testing.T) {
	for _, k := range []string{"password", "PASSWORD", "api-key", "refresh_token", "X-Authorization"} {
		if !IsSecretKey(k) {
			t.Errorf("IsSecretKey(%q) = false, mau true", k)
		}
	}
	for _, k := range []string{"email", "player_id", "slot", "score"} {
		if IsSecretKey(k) {
			t.Errorf("IsSecretKey(%q) = true, mau false", k)
		}
	}
}
