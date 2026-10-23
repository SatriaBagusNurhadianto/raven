//go:build integration

package tenant

import (
	"context"
	"os"
	"testing"

	"github.com/chava/raven/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTenantResolveByAPIKey(t *testing.T) {
	dsn := os.Getenv("RAVEN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("RAVEN_TEST_DATABASE_URL tidak diisi")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	dbConn := &db.DB{Pool: pool}
	mig, _ := db.NewDefaultMigrator(dbConn)
	if _, err := mig.Up(ctx); err != nil {
		t.Fatal(err)
	}
	_, _ = pool.Exec(ctx, "delete from games")
	const key = "raven_test_key_123"
	_, err = pool.Exec(ctx, `insert into games (name,slug,api_key_hash,api_key_prefix,secret_hash)
		values ('g','g',$1,'p','s')`, hashKey(key))
	if err != nil {
		t.Fatal(err)
	}

	svc := NewService(pool)
	g, err := svc.ResolveByAPIKey(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if g.Slug != "g" {
		t.Errorf("slug = %s", g.Slug)
	}
	_, err = svc.ResolveByAPIKey(ctx, "wrong")
	if err == nil {
		t.Fatal("expected error for wrong key")
	}
}
