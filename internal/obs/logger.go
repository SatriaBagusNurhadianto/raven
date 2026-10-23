// Package obs menyediakan logging terstruktur dan metrik Prometheus untuk Raven.
package obs

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

// ParseLevel mengubah string level menjadi slog.Level. Nilai tak dikenal → LevelInfo.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewLogger membuat logger JSON terstruktur ke w (PRD §6: log terstruktur JSON).
// Semua atribut yang namanya menyerupai rahasia otomatis disunting.
func NewLogger(w io.Writer, level string) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: ParseLevel(level)})
	return slog.New(NewRedactingHandler(h))
}

// Discard mengembalikan logger yang membuang semua output (dipakai di test).
func Discard() *slog.Logger {
	return slog.New(NewRedactingHandler(slog.NewJSONHandler(io.Discard, nil)))
}

var secretKeys = []string{
	"password", "passwd", "secret", "token", "authorization",
	"api_key", "apikey", "private_key", "refresh", "cookie", "dsn",
}

// IsSecretKey melaporkan apakah nama field berpotensi berisi rahasia.
func IsSecretKey(key string) bool {
	k := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, s := range secretKeys {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// RedactingHandler membungkus slog.Handler dan mengganti nilai field rahasia
// dengan "[redacted]" supaya secret tidak pernah masuk log (PRD §8).
type RedactingHandler struct{ next slog.Handler }

// NewRedactingHandler membungkus handler berikutnya dengan penyuntingan rahasia.
func NewRedactingHandler(next slog.Handler) *RedactingHandler {
	return &RedactingHandler{next: next}
}

// Enabled meneruskan pemeriksaan level ke handler berikutnya.
func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle menyunting atribut rahasia sebelum diteruskan.
func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(redactAttr(a))
		return true
	})
	return h.next.Handle(ctx, out)
}

// WithAttrs menyunting atribut bawaan yang menempel di logger.
func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	red := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		red = append(red, redactAttr(a))
	}
	return &RedactingHandler{next: h.next.WithAttrs(red)}
}

// WithGroup meneruskan grup ke handler berikutnya.
func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{next: h.next.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	if IsSecretKey(a.Key) {
		return slog.String(a.Key, "[redacted]")
	}
	if a.Value.Kind() == slog.KindGroup {
		grp := a.Value.Group()
		out := make([]any, 0, len(grp))
		for _, g := range grp {
			out = append(out, redactAttr(g))
		}
		return slog.Group(a.Key, out...)
	}
	return a
}
