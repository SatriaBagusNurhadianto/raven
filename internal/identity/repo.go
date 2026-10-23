package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound dipakai bila entitas tidak ditemukan.
var ErrNotFound = errors.New("tidak ditemukan")

// Store mengakses tabel players/identities/sessions.
type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

// --- Player ---

func (s *Store) CreatePlayer(ctx context.Context, gameID, status, displayName string) (*Player, error) {
	var p Player
	var bannedAt *time.Time
	var banReason *string
	var dn *string
	err := s.Pool.QueryRow(ctx, `
		insert into players (game_id, status, display_name)
		values ($1,$2,$3)
		returning id, game_id, status, display_name, banned_at, ban_reason, created_at, last_seen`,
		gameID, status, nullIfEmpty(displayName)).Scan(
		&p.ID, &p.GameID, &p.Status, &dn, &bannedAt, &banReason, &p.CreatedAt, &p.LastSeen)
	if err != nil {
		return nil, fmt.Errorf("create player: %w", err)
	}
	if dn != nil {
		p.DisplayName = *dn
	}
	p.BannedAt = bannedAt
	if banReason != nil {
		p.BanReason = *banReason
	}
	return &p, nil
}

func (s *Store) GetPlayer(ctx context.Context, id string) (*Player, error) {
	var p Player
	var bannedAt *time.Time
	var banReason *string
	var displayName *string
	err := s.Pool.QueryRow(ctx, `
		select id, game_id, status, display_name, banned_at, ban_reason, created_at, last_seen
		from players where id=$1`, id).Scan(
		&p.ID, &p.GameID, &p.Status, &displayName, &bannedAt, &banReason, &p.CreatedAt, &p.LastSeen)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get player: %w", err)
	}
	if displayName != nil {
		p.DisplayName = *displayName
	}
	p.BannedAt = bannedAt
	if banReason != nil {
		p.BanReason = *banReason
	}
	return &p, nil
}

func (s *Store) UpdatePlayerStatus(ctx context.Context, id, status, reason string) error {
	var bannedAt any
	if status == StatusBanned {
		bannedAt = time.Now().UTC()
	} else {
		bannedAt = nil
	}
	_, err := s.Pool.Exec(ctx, `
		update players set status=$2, banned_at=$3, ban_reason=$4, last_seen=now() where id=$1`,
		id, status, bannedAt, nullIfEmpty(reason))
	return err
}

func (s *Store) TouchLastSeen(ctx context.Context, id string) {
	_, _ = s.Pool.Exec(ctx, "update players set last_seen=now() where id=$1", id)
}

func (s *Store) DeletePlayer(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, "delete from players where id=$1", id)
	return err
}

// --- Identity ---

func (s *Store) CreateIdentity(ctx context.Context, in Identity) (*Identity, error) {
	var out Identity
	var emailVerified *time.Time
	var passwordHash *string
	err := s.Pool.QueryRow(ctx, `
		insert into identities (player_id, game_id, provider, external_id, password_hash, email_verified_at)
		values ($1,$2,$3,$4,$5,$6)
		returning id, player_id, game_id, provider, external_id, password_hash, email_verified_at, created_at`,
		in.PlayerID, in.GameID, in.Provider, in.ExternalID, nullIfEmpty(in.PasswordHash), in.EmailVerified).Scan(
		&out.ID, &out.PlayerID, &out.GameID, &out.Provider, &out.ExternalID, &passwordHash, &emailVerified, &out.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create identity: %w", err)
	}
	if passwordHash != nil {
		out.PasswordHash = *passwordHash
	}
	out.EmailVerified = emailVerified
	return &out, nil
}

func (s *Store) GetIdentityByProvider(ctx context.Context, gameID, provider, externalID string) (*Identity, error) {
	var out Identity
	var emailVerified *time.Time
	var passwordHash *string
	err := s.Pool.QueryRow(ctx, `
		select id, player_id, game_id, provider, external_id, password_hash, email_verified_at, created_at
		from identities where game_id=$1 and provider=$2 and external_id=$3`,
		gameID, provider, externalID).Scan(
		&out.ID, &out.PlayerID, &out.GameID, &out.Provider, &out.ExternalID, &passwordHash, &emailVerified, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get identity: %w", err)
	}
	if passwordHash != nil {
		out.PasswordHash = *passwordHash
	}
	out.EmailVerified = emailVerified
	return &out, nil
}

func (s *Store) GetIdentitiesByPlayer(ctx context.Context, playerID string) ([]Identity, error) {
	rows, err := s.Pool.Query(ctx, `
		select id, player_id, game_id, provider, external_id, password_hash, email_verified_at, created_at
		from identities where player_id=$1`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Identity
	for rows.Next() {
		var in Identity
		var emailVerified *time.Time
		var passwordHash *string
		if err := rows.Scan(&in.ID, &in.PlayerID, &in.GameID, &in.Provider, &in.ExternalID, &passwordHash, &emailVerified, &in.CreatedAt); err != nil {
			return nil, err
		}
		if passwordHash != nil {
			in.PasswordHash = *passwordHash
		}
		in.EmailVerified = emailVerified
		out = append(out, in)
	}
	return out, rows.Err()
}

func (s *Store) HasPasswordIdentity(ctx context.Context, playerID string) (bool, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `
		select count(*) from identities where player_id=$1 and provider='email'`, playerID).Scan(&n)
	return n > 0, err
}

// --- Session ---

func (s *Store) CreateSession(ctx context.Context, in Session) (*Session, error) {
	var out Session
	var revokedAt *time.Time
	var rotatedTo *string
	var lastUsed *time.Time
	var deviceID, userAgent, ip *string
	var revokedReason *string
	err := s.Pool.QueryRow(ctx, `
		insert into sessions (player_id, game_id, family_id, refresh_hash, device_id, user_agent, ip, expires_at)
		values ($1,$2,$3,$4,$5,$6,$7,$8)
		returning id, player_id, game_id, family_id, refresh_hash, device_id, user_agent, ip::text, expires_at, revoked_at, revoked_reason, rotated_to, created_at, last_used_at`,
		in.PlayerID, in.GameID, in.FamilyID, in.RefreshHash, nullIfEmpty(in.DeviceID), nullIfEmpty(in.UserAgent), nullIfEmpty(in.IP), in.ExpiresAt).Scan(
		&out.ID, &out.PlayerID, &out.GameID, &out.FamilyID, &out.RefreshHash, &deviceID, &userAgent, &ip, &out.ExpiresAt, &revokedAt, &revokedReason, &rotatedTo, &out.CreatedAt, &lastUsed)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	if deviceID != nil {
		out.DeviceID = *deviceID
	}
	if userAgent != nil {
		out.UserAgent = *userAgent
	}
	if ip != nil {
		out.IP = *ip
	}
	out.RevokedAt = revokedAt
	out.LastUsedAt = lastUsed
	if revokedReason != nil {
		out.RevokedReason = *revokedReason
	}
	if rotatedTo != nil {
		out.RotatedTo = *rotatedTo
	}
	return &out, nil
}

func (s *Store) GetSessionByRefresh(ctx context.Context, refreshHash string) (*Session, error) {
	var out Session
	var revokedAt *time.Time
	var rotatedTo *string
	var lastUsed *time.Time
	var deviceID, userAgent, ip *string
	var revokedReason *string
	err := s.Pool.QueryRow(ctx, `
		select id, player_id, game_id, family_id, refresh_hash, device_id, user_agent, ip::text, expires_at, revoked_at, revoked_reason, rotated_to, created_at, last_used_at
		from sessions where refresh_hash=$1`, refreshHash).Scan(
		&out.ID, &out.PlayerID, &out.GameID, &out.FamilyID, &out.RefreshHash, &deviceID, &userAgent, &ip, &out.ExpiresAt, &revokedAt, &revokedReason, &rotatedTo, &out.CreatedAt, &lastUsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if deviceID != nil {
		out.DeviceID = *deviceID
	}
	if userAgent != nil {
		out.UserAgent = *userAgent
	}
	if ip != nil {
		out.IP = *ip
	}
	out.RevokedAt = revokedAt
	out.LastUsedAt = lastUsed
	if revokedReason != nil {
		out.RevokedReason = *revokedReason
	}
	if rotatedTo != nil {
		out.RotatedTo = *rotatedTo
	}
	return &out, nil
}

// RevokeSession menandai satu session sebagai dicabut.
func (s *Store) RevokeSession(ctx context.Context, id, reason string) error {
	_, err := s.Pool.Exec(ctx, `
		update sessions set revoked_at=now(), revoked_reason=$2 where id=$1 and revoked_at is null`,
		id, nullIfEmpty(reason))
	return err
}

// RevokeFamily mencabut seluruh family token (deteksi reuse, FR-A5).
func (s *Store) RevokeFamily(ctx context.Context, familyID, reason string) error {
	_, err := s.Pool.Exec(ctx, `
		update sessions set revoked_at=now(), revoked_reason=$2
		where family_id=$1 and revoked_at is null`, familyID, nullIfEmpty(reason))
	return err
}

// RevokeAllForPlayer mencabut semua session aktif player (logout semua perangkat).
func (s *Store) RevokeAllForPlayer(ctx context.Context, playerID, reason string) error {
	_, err := s.Pool.Exec(ctx, `
		update sessions set revoked_at=now(), revoked_reason=$2
		where player_id=$1 and revoked_at is null`, playerID, nullIfEmpty(reason))
	return err
}

// RotateSession menandai session lama rotated_to session baru.
func (s *Store) RotateSession(ctx context.Context, oldID, newID string) error {
	_, err := s.Pool.Exec(ctx, `
		update sessions set revoked_at=now(), revoked_reason='rotated', rotated_to=$2
		where id=$1 and revoked_at is null`, oldID, newID)
	return err
}

// TouchSession memperbarui last_used_at.
func (s *Store) TouchSession(ctx context.Context, id string) {
	_, _ = s.Pool.Exec(ctx, "update sessions set last_used_at=now() where id=$1", id)
}

// CountActiveSessions menghitung session aktif player.
func (s *Store) CountActiveSessions(ctx context.Context, playerID string) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `
		select count(*) from sessions where player_id=$1 and revoked_at is null and expires_at > now()`,
		playerID).Scan(&n)
	return n, err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
