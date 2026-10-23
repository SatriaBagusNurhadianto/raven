//go:build integration

package backoffice

import (
	"context"
	"os"
	"testing"

	"github.com/chava/raven/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBackofficeAdminLoginAndAudit(t *testing.T) {
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
	_, _ = pool.Exec(ctx, "delete from audit_logs")
	_, _ = pool.Exec(ctx, "delete from admin_users")

	svc := NewService(pool)
	u, err := svc.CreateAdmin(ctx, "admin@test.com", "secret123", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !svc.HasRole(u, "viewer") {
		t.Fatal("admin should have viewer role")
	}
	logged, err := svc.Login(ctx, "admin@test.com", "secret123")
	if err != nil {
		t.Fatal(err)
	}
	if logged.ID != u.ID {
		t.Errorf("login id mismatch")
	}
	_, err = svc.Login(ctx, "admin@test.com", "wrong")
	if err == nil {
		t.Fatal("expected invalid creds")
	}
	if err := svc.Audit(ctx, u.ID, "ban_player", "player", "p1", nil, map[string]any{"reason": "cheat"}); err != nil {
		t.Fatal(err)
	}
	var cnt int
	_ = pool.QueryRow(ctx, "select count(*) from audit_logs").Scan(&cnt)
	if cnt != 1 {
		t.Errorf("audit count = %d", cnt)
	}
}
