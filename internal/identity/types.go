// Package identity menyediakan autentikasi pemain (guest, email, refresh token,
// linking, ban) untuk Raven (PRD FR-A1..A6).
package identity

import (
	"context"
	"time"
)

// Status pemain.
const (
	StatusActive  = "active"
	StatusBanned  = "banned"
	StatusDeleted = "deleted"
)

// Player adalah entitas pemain per game.
type Player struct {
	ID          string
	GameID      string
	Status      string
	DisplayName string
	BannedAt    *time.Time
	BanReason   string
	CreatedAt   time.Time
	LastSeen    time.Time
}

// Identity menghubungkan player dengan provider (guest device, email, OAuth).
type Identity struct {
	ID            string
	PlayerID      string
	GameID        string
	Provider      string
	ExternalID    string
	PasswordHash  string
	EmailVerified *time.Time
	CreatedAt     time.Time
}

// Session adalah satu refresh token family member.
type Session struct {
	ID            string
	PlayerID      string
	GameID        string
	FamilyID      string
	RefreshHash   string
	DeviceID      string
	UserAgent     string
	IP            string
	ExpiresAt     time.Time
	RevokedAt     *time.Time
	RevokedReason string
	RotatedTo     string
	CreatedAt     time.Time
	LastUsedAt    *time.Time
}

// TokenPair adalah pasangan access+refresh token yang dikembalikan ke SDK.
type TokenPair struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	ExpiresIn    int     `json:"expires_in"`
	TokenType    string  `json:"token_type"`
	Player       *Player `json:"player"`
}

// Claims adalah payload JWT access token.
type Claims struct {
	PlayerID string `json:"sub"`
	GameID   string `json:"game"`
	JTI      string `json:"jti"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
}

// ContextKey untuk menyisipkan identitas ke context request.
type ctxKey int

const (
	CtxPlayerID ctxKey = iota
	CtxGameID
	CtxSessionID
)

// PlayerIDFrom mengambil player ID dari context.
func PlayerIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(CtxPlayerID).(string)
	return v
}

// GameIDFrom mengambil game ID dari context.
func GameIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(CtxGameID).(string)
	return v
}

// SessionIDFrom mengambil session ID dari context.
func SessionIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(CtxSessionID).(string)
	return v
}
