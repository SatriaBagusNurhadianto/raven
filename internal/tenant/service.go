package tenant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidKey = errors.New("api key tidak valid")

type Game struct {
	ID       string
	Name     string
	Slug     string
	Settings map[string]any
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

func (s *Service) ResolveByAPIKey(ctx context.Context, apiKey string) (*Game, error) {
	g := &Game{}
	var settings []byte
	err := s.pool.QueryRow(ctx, `
		select id, name, slug, settings from games
		where api_key_hash = $1 and disabled_at is null`, hashKey(apiKey)).
		Scan(&g.ID, &g.Name, &g.Slug, &settings)
	if err != nil {
		return nil, ErrInvalidKey
	}
	return g, nil
}
