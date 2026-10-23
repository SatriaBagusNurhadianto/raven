package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// DecodeJSON membaca body JSON ke dst dengan batas ukuran dan validasi dasar.
// Semua kegagalan dikembalikan sebagai *Problem siap kirim.
func DecodeJSON(r *http.Request, maxBytes int64, dst any) *Problem {
	if r.Body == nil {
		return BadRequest("Body request kosong; JSON wajib dikirim.")
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		mediaType := strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
		if mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
			return UnsupportedMediaType("Content-Type harus application/json, dapat " + mediaType + ".")
		}
	}

	// Body dibaca berbatas ke memori agar overflow terdeteksi andal: decoder JSON
	// bisa melaporkan "unexpected EOF" (bukan MaxBytesError) saat input terpotong.
	// Batas besar (blob save) ditangani handler khusus, bukan di sini.
	reader := io.Reader(r.Body)
	if maxBytes > 0 {
		reader = io.LimitReader(r.Body, maxBytes+1)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return PayloadTooLarge("Body melebihi batas " + itoa64(maxBytes) + " byte.")
		}
		return BadRequest("Gagal membaca body: " + err.Error())
	}
	if maxBytes > 0 && int64(len(raw)) > maxBytes {
		return PayloadTooLarge("Body melebihi batas " + itoa64(maxBytes) + " byte.")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return BadRequest("Body request kosong; JSON wajib dikirim.")
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return BadRequest("Body request kosong; JSON wajib dikirim.")
		}
		return BadRequest("JSON tidak valid: " + err.Error())
	}

	// Tolak payload yang berisi lebih dari satu nilai JSON.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return BadRequest("Body hanya boleh memuat satu objek JSON.")
	}
	return nil
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
