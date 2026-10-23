package backoffice

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
)

var ErrInvalidCreds = errors.New("kredensial tidak valid")

type AdminUser struct {
	ID    string
	Email string
	Role  string
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func hashPassword(pw string) string {
	salt := []byte("raven_salt_v1")
	return hex.EncodeToString(argon2.IDKey([]byte(pw), salt, 1, 64*1024, 4, 32))
}

func (s *Service) CreateAdmin(ctx context.Context, email, password, role string) (*AdminUser, error) {
	if role == "" {
		role = "viewer"
	}
	h := hashPassword(password)
	var id string
	err := s.pool.QueryRow(ctx, `
		insert into admin_users (email, password_hash, role) values ($1, $2, $3) returning id`,
		email, string(h), role).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &AdminUser{ID: id, Email: email, Role: role}, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (*AdminUser, error) {
	var id, role, hash string
	err := s.pool.QueryRow(ctx, `
		select id, role, password_hash from admin_users where email=$1 and disabled_at is null`, email).
		Scan(&id, &role, &hash)
	if err != nil {
		return nil, ErrInvalidCreds
	}
	if hashPassword(password) != hash {
		return nil, ErrInvalidCreds
	}
	_, _ = s.pool.Exec(ctx, `update admin_users set last_login_at=now() where id=$1`, id)
	return &AdminUser{ID: id, Email: email, Role: role}, nil
}

func (s *Service) Audit(ctx context.Context, actorID, action, targetType, targetID string, gameID *string, meta map[string]any) error {
	_, err := s.pool.Exec(ctx, `
		insert into audit_logs (actor_type, actor_id, action, target_type, target_id, game_id, metadata)
		values ('admin', $1, $2, $3, $4, $5, $6)`,
		actorID, action, targetType, targetID, gameID, meta)
	return err
}

func (s *Service) HasRole(u *AdminUser, minRole string) bool {
	rank := map[string]int{"viewer": 1, "admin": 2, "owner": 3}
	return rank[u.Role] >= rank[minRole]
}
