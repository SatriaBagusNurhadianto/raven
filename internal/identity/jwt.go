package identity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// JWTIssuer menandatangani dan memverifikasi access token (HS256).
type JWTIssuer struct {
	Secret []byte
	TTL    time.Duration
	Now    func() time.Time
}

func (j *JWTIssuer) now() time.Time {
	if j.Now != nil {
		return j.Now()
	}
	return time.Now()
}

// Issue membuat access token JWT untuk player.
func (j *JWTIssuer) Issue(playerID, gameID string) (string, int, error) {
	if len(j.Secret) == 0 {
		return "", 0, fmt.Errorf("jwt secret kosong")
	}
	now := j.now().UTC()
	claims := Claims{
		PlayerID: playerID,
		GameID:   gameID,
		JTI:      newJTI(),
		IssuedAt: now.Unix(),
		Expires:  now.Add(j.TTL).Unix(),
	}
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	hEnc := base64.RawURLEncoding.EncodeToString(hb)
	cEnc := base64.RawURLEncoding.EncodeToString(cb)
	sig := j.sign(hEnc + "." + cEnc)
	return hEnc + "." + cEnc + "." + sig, int(j.TTL.Seconds()), nil
}

// Verify memvalidasi token dan mengembalikan claims. Mengembalikan error bila
// tanda tangan salah atau token kadaluarsa.
func (j *JWTIssuer) Verify(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("format token tidak valid")
	}
	// verify alg header
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}
	var hdr map[string]string
	if err := json.Unmarshal(hb, &hdr); err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}
	if hdr["alg"] != "HS256" {
		return nil, fmt.Errorf("alg tidak didukung: %s", hdr["alg"])
	}
	if j.sign(parts[0]+"."+parts[1]) != parts[2] {
		return nil, fmt.Errorf("tanda tangan token tidak valid")
	}
	cb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}
	var c Claims
	if err := json.Unmarshal(cb, &c); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}
	if time.Unix(c.Expires, 0).Before(j.now()) {
		return nil, fmt.Errorf("token kadaluarsa")
	}
	if c.PlayerID == "" || c.GameID == "" {
		return nil, fmt.Errorf("claims tidak lengkap")
	}
	return &c, nil
}

func (j *JWTIssuer) sign(input string) string {
	mac := hmac.New(sha256.New, j.Secret)
	mac.Write([]byte(input))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func newJTI() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "jti-unavailable"
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
