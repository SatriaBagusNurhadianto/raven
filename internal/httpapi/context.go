package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
)

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyClientIP
	ctxKeyRoute
)

const headerRequestID = "X-Request-Id"

// NewRequestID membuat ID request acak 16 byte (32 karakter heksadesimal).
func NewRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand gagal berarti ada masalah serius di sistem; tetap
		// kembalikan nilai unik agar request tidak kehilangan korelasi.
		return "req-unavailable"
	}
	return hex.EncodeToString(b[:])
}

// WithRequestID menyisipkan request ID ke context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID, id)
}

// RequestIDFrom mengambil request ID dari context (kosong bila tidak ada).
func RequestIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(ctxKeyRequestID).(string)
	return id
}

// WithClientIP menyisipkan IP klien ke context.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, ctxKeyClientIP, ip)
}

// ClientIPFrom mengambil IP klien dari context.
func ClientIPFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	ip, _ := ctx.Value(ctxKeyClientIP).(string)
	return ip
}

// WithRoute menyisipkan nama route (pola) ke context untuk label metrik.
func WithRoute(ctx context.Context, route string) context.Context {
	return context.WithValue(ctx, ctxKeyRoute, route)
}

// RouteFrom mengambil nama route dari context.
func RouteFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	route, _ := ctx.Value(ctxKeyRoute).(string)
	if route == "" {
		return "unmatched"
	}
	return route
}

// IsValidRequestID memvalidasi ID yang dikirim klien supaya tidak bisa dipakai
// untuk log injection: hanya [A-Za-z0-9._-], panjang 8..128.
func IsValidRequestID(id string) bool {
	if len(id) < 8 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}

// ClientIP menentukan IP klien dengan mempertimbangkan trusted proxy.
// Header X-Forwarded-For hanya dipercaya bila peer langsung ada di daftar trusted.
func ClientIP(r *http.Request, trusted []string) string {
	peer := hostOnly(r.RemoteAddr)
	if peer == "" {
		return ""
	}
	if len(trusted) == 0 || !ipAllowed(peer, trusted) {
		return peer
	}
	// Telusuri XFF dari kanan: entri pertama yang bukan trusted proxy adalah klien.
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return peer
	}
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		cand := strings.TrimSpace(parts[i])
		if cand == "" {
			continue
		}
		if !ipAllowed(cand, trusted) {
			return cand
		}
	}
	return peer
}

func hostOnly(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}

// ipAllowed melaporkan apakah ip cocok dengan salah satu entri trusted
// (CIDR atau alamat tunggal).
func ipAllowed(ip string, trusted []string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, entry := range trusted {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, cidr, err := net.ParseCIDR(entry); err == nil && cidr.Contains(parsed) {
				return true
			}
			continue
		}
		if t := net.ParseIP(entry); t != nil && t.Equal(parsed) {
			return true
		}
	}
	return false
}
