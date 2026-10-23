package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/chava/raven/internal/httpapi"
)

// Service mengorkestrasi alur auth (PRD FR-A1..A6).
type Service struct {
	Store      *Store
	JWT        *JWTIssuer
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	Now        func() time.Time

	// BanCacheTTL mengatur seberapa sering status ban dicek ulang (FR-A6: <=60s).
	BanCacheTTL time.Duration
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

// Guest membuat akun anonim dari device ID (FR-A1).
func (s *Service) Guest(ctx context.Context, gameID, deviceID, userAgent, ip string) (*TokenPair, error) {
	if strings.TrimSpace(deviceID) == "" {
		return nil, httpapi.BadRequest("device_id wajib diisi.")
	}
	// Cek apakah device ini sudah punya akun guest.
	if ident, err := s.Store.GetIdentityByProvider(ctx, gameID, "guest", deviceID); err == nil {
		player, err := s.Store.GetPlayer(ctx, ident.PlayerID)
		if err != nil {
			return nil, err
		}
		if player.Status == StatusBanned {
			return nil, httpapi.Banned("Pemain diblokir.")
		}
		return s.issueTokens(ctx, player, gameID, userAgent, ip)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	player, err := s.Store.CreatePlayer(ctx, gameID, StatusActive, "")
	if err != nil {
		return nil, err
	}
	if _, err := s.Store.CreateIdentity(ctx, Identity{
		PlayerID:   player.ID,
		GameID:     gameID,
		Provider:   "guest",
		ExternalID: deviceID,
	}); err != nil {
		return nil, err
	}
	return s.issueTokens(ctx, player, gameID, userAgent, ip)
}

// Register membuat akun email+password (FR-A2).
func (s *Service) Register(ctx context.Context, gameID, email, password, userAgent, ip string) (*TokenPair, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !validEmail(email) {
		return nil, httpapi.Validation("Email tidak valid.", httpapi.FieldError{Field: "email", Detail: "format email tidak valid"})
	}
	if len(password) < 8 {
		return nil, httpapi.Validation("Password terlalu pendek.", httpapi.FieldError{Field: "password", Detail: "minimal 8 karakter"})
	}
	// Cek duplikat.
	if _, err := s.Store.GetIdentityByProvider(ctx, gameID, "email", email); err == nil {
		return nil, httpapi.Conflict("Email sudah terdaftar.")
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	player, err := s.Store.CreatePlayer(ctx, gameID, StatusActive, email)
	if err != nil {
		return nil, err
	}
	if _, err := s.Store.CreateIdentity(ctx, Identity{
		PlayerID:     player.ID,
		GameID:       gameID,
		Provider:     "email",
		ExternalID:   email,
		PasswordHash: hash,
	}); err != nil {
		return nil, err
	}
	return s.issueTokens(ctx, player, gameID, userAgent, ip)
}

// Login memverifikasi email+password (FR-A2).
func (s *Service) Login(ctx context.Context, gameID, email, password, userAgent, ip string) (*TokenPair, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	ident, err := s.Store.GetIdentityByProvider(ctx, gameID, "email", email)
	if errors.Is(err, ErrNotFound) {
		return nil, httpapi.Unauthorized("Email atau password salah.")
	}
	if err != nil {
		return nil, err
	}
	if !VerifyPassword(password, ident.PasswordHash) {
		return nil, httpapi.Unauthorized("Email atau password salah.")
	}
	player, err := s.Store.GetPlayer(ctx, ident.PlayerID)
	if err != nil {
		return nil, err
	}
	if player.Status == StatusBanned {
		return nil, httpapi.Banned("Pemain diblokir.")
	}
	if player.Status == StatusDeleted {
		return nil, httpapi.Unauthorized("Akun tidak aktif.")
	}
	return s.issueTokens(ctx, player, gameID, userAgent, ip)
}

// Link menghubungkan akun guest ke email+password tanpa kehilangan data (FR-A3).
func (s *Service) Link(ctx context.Context, playerID, gameID, email, password string) error {
	player, err := s.Store.GetPlayer(ctx, playerID)
	if err != nil {
		return err
	}
	if player.GameID != gameID {
		return httpapi.Forbidden("Player tidak termasuk game ini.")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !validEmail(email) {
		return httpapi.Validation("Email tidak valid.", httpapi.FieldError{Field: "email", Detail: "format email tidak valid"})
	}
	if len(password) < 8 {
		return httpapi.Validation("Password terlalu pendek.", httpapi.FieldError{Field: "password", Detail: "minimal 8 karakter"})
	}
	if _, err := s.Store.GetIdentityByProvider(ctx, gameID, "email", email); err == nil {
		return httpapi.Conflict("Email sudah terdaftar.")
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	if _, err := s.Store.CreateIdentity(ctx, Identity{
		PlayerID:     player.ID,
		GameID:       gameID,
		Provider:     "email",
		ExternalID:   email,
		PasswordHash: hash,
	}); err != nil {
		return err
	}
	return nil
}

// Refresh menukar refresh token lama dengan pasangan baru (rotasi, FR-A1/A5).
func (s *Service) Refresh(ctx context.Context, refreshToken, userAgent, ip string) (*TokenPair, error) {
	hash := hashRefresh(refreshToken)
	sess, err := s.Store.GetSessionByRefresh(ctx, hash)
	if errors.Is(err, ErrNotFound) {
		return nil, httpapi.Unauthorized("Refresh token tidak valid.")
	}
	if err != nil {
		return nil, err
	}
	// Reuse detection: token yang sudah dicabut dipakai lagi -> cabut seluruh family.
	if sess.RevokedAt != nil {
		_ = s.Store.RevokeFamily(ctx, sess.FamilyID, "reuse_detected")
		return nil, httpapi.Unauthorized("Refresh token sudah dipakai ulang; seluruh sesi dicabut.")
	}
	if sess.ExpiresAt.Before(s.now()) {
		_ = s.Store.RevokeSession(ctx, sess.ID, "expired")
		return nil, httpapi.Unauthorized("Refresh token kadaluarsa.")
	}
	player, err := s.Store.GetPlayer(ctx, sess.PlayerID)
	if err != nil {
		return nil, err
	}
	if player.Status == StatusBanned {
		_ = s.Store.RevokeAllForPlayer(ctx, player.ID, "banned")
		return nil, httpapi.Banned("Pemain diblokir.")
	}
	if player.Status == StatusDeleted {
		return nil, httpapi.Unauthorized("Akun tidak aktif.")
	}

	// Terbitkan session baru (rotasi).
	newRefresh, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	newSess, err := s.Store.CreateSession(ctx, Session{
		PlayerID:    sess.PlayerID,
		GameID:      sess.GameID,
		FamilyID:    sess.FamilyID,
		RefreshHash: hashRefresh(newRefresh),
		DeviceID:    "",
		UserAgent:   userAgent,
		IP:          ip,
		ExpiresAt:   s.now().Add(s.RefreshTTL),
	})
	if err != nil {
		return nil, err
	}
	if err := s.Store.RotateSession(ctx, sess.ID, newSess.ID); err != nil {
		return nil, err
	}
	s.Store.TouchSession(ctx, newSess.ID)

	access, expiresIn, err := s.JWT.Issue(player.ID, sess.GameID)
	if err != nil {
		return nil, err
	}
	s.Store.TouchLastSeen(ctx, player.ID)
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: newRefresh,
		ExpiresIn:    expiresIn,
		TokenType:    "Bearer",
		Player:       player,
	}, nil
}

// Logout mencabut satu session (FR-A4).
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	hash := hashRefresh(refreshToken)
	sess, err := s.Store.GetSessionByRefresh(ctx, hash)
	if errors.Is(err, ErrNotFound) {
		return nil // idempoten
	}
	if err != nil {
		return err
	}
	return s.Store.RevokeSession(ctx, sess.ID, "logout")
}

// LogoutAll mencabut semua session player (FR-A4).
func (s *Service) LogoutAll(ctx context.Context, playerID string) error {
	return s.Store.RevokeAllForPlayer(ctx, playerID, "logout_all")
}

// Ban memblokir player dan mencabut semua sessionnya (FR-A6).
func (s *Service) Ban(ctx context.Context, playerID, reason string) error {
	if err := s.Store.UpdatePlayerStatus(ctx, playerID, StatusBanned, reason); err != nil {
		return err
	}
	return s.Store.RevokeAllForPlayer(ctx, playerID, "banned")
}

// Unban mengaktifkan kembali player.
func (s *Service) Unban(ctx context.Context, playerID string) error {
	return s.Store.UpdatePlayerStatus(ctx, playerID, StatusActive, "")
}

// IsBanned memeriksa status ban (dipakai middleware, FR-A6 <=60s).
func (s *Service) IsBanned(ctx context.Context, playerID string) (bool, error) {
	p, err := s.Store.GetPlayer(ctx, playerID)
	if err != nil {
		return false, err
	}
	return p.Status == StatusBanned, nil
}

// issueTokens menerbitkan access+refresh untuk player yang sudah valid.
func (s *Service) issueTokens(ctx context.Context, player *Player, gameID, userAgent, ip string) (*TokenPair, error) {
	access, expiresIn, err := s.JWT.Issue(player.ID, gameID)
	if err != nil {
		return nil, err
	}
	refresh, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	familyID := newFamilyID()
	sess, err := s.Store.CreateSession(ctx, Session{
		PlayerID:    player.ID,
		GameID:      gameID,
		FamilyID:    familyID,
		RefreshHash: hashRefresh(refresh),
		UserAgent:   userAgent,
		IP:          ip,
		ExpiresAt:   s.now().Add(s.RefreshTTL),
	})
	if err != nil {
		return nil, err
	}
	s.Store.TouchSession(ctx, sess.ID)
	s.Store.TouchLastSeen(ctx, player.ID)
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    expiresIn,
		TokenType:    "Bearer",
		Player:       player,
	}, nil
}

// --- helpers ---

func newRefreshToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func newFamilyID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "family-unavailable"
	}
	return hex.EncodeToString(b[:])
}

func hashRefresh(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func validEmail(email string) bool {
	if len(email) < 5 || len(email) > 254 {
		return false
	}
	at := -1
	for i := 0; i < len(email); i++ {
		if email[i] == '@' {
			if at >= 0 {
				return false
			}
			at = i
		}
	}
	if at < 1 || at == len(email)-1 {
		return false
	}
	dot := strings.LastIndex(email[at:], ".")
	return dot > 1 && at+dot < len(email)-1
}
