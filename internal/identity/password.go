package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parameter Argon2id (OWASP 2024 recommendation, disesuaikan untuk server game).
const (
	argonTime    = 2
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword menghasilkan string PHC (Argon2id) yang menyimpan salt + params.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("baca salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	// Format: $argon2id$v=19$m=65536,t=2,p=4$<salt>$<key>
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword memverifikasi password terhadap hash PHC. Menggunakan
// perbandingan waktu-konstan untuk mencegah timing attack.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	// parts: ["", "argon2id", "v=19", "m=...,t=...,p=...", salt, key]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var memory uint32
	var timeCost uint32
	var threads uint8
	for _, kv := range strings.Split(parts[3], ",") {
		k, v, ok := splitKV(kv)
		if !ok {
			return false
		}
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return false
		}
		switch k {
		case "m":
			if n > 1<<32-1 {
				return false
			}
			memory = uint32(n)
		case "t":
			if n > 1<<32-1 {
				return false
			}
			timeCost = uint32(n)
		case "p":
			if n > 255 {
				return false
			}
			threads = uint8(n)
		}
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	key := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(expected))) // #nosec G115
	return subtle.ConstantTimeCompare(key, expected) == 1
}

func splitKV(s string) (string, string, bool) {
	i := strings.IndexByte(s, '=')
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}
