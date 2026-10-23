//go:build integration

package identity

import (
	"context"
	"os"
	"testing"

	"github.com/chava/raven/internal/config"
	"github.com/chava/raven/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupIdentityTest(t *testing.T) (*Service, string, func()) {
	t.Helper()
	dsn := os.Getenv("RAVEN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("RAVEN_TEST_DATABASE_URL tidak diisi")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	dbConn := &db.DB{Pool: pool}
	mig, _ := db.NewDefaultMigrator(dbConn)
	if _, err := mig.Up(ctx); err != nil {
		t.Fatal(err)
	}
	// bersihkan
	_, _ = pool.Exec(ctx, "delete from sessions")
	_, _ = pool.Exec(ctx, "delete from identities")
	_, _ = pool.Exec(ctx, "delete from players")
	_, _ = pool.Exec(ctx, "delete from games")

	// buat game
	var gameID string
	err = pool.QueryRow(ctx, `
		insert into games (name, slug, api_key_hash, api_key_prefix, secret_hash)
		values ('testgame','testgame','hash','pref','secret') returning id`).Scan(&gameID)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Defaults()
	cfg.JWTSecret = "test-secret-at-least-32-chars-long-xxxx"
	jwt := &JWTIssuer{Secret: []byte(cfg.JWTSecret), TTL: cfg.AccessTokenTTL}
	store := NewStore(pool)
	svc := &Service{Store: store, JWT: jwt, AccessTTL: cfg.AccessTokenTTL, RefreshTTL: cfg.RefreshTokenTTL}
	return svc, gameID, func() { pool.Close() }
}

func TestGuestFlow(t *testing.T) {
	svc, gameID, cleanup := setupIdentityTest(t)
	defer cleanup()
	ctx := context.Background()

	tp, err := svc.Guest(ctx, gameID, "device-1", "ua", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if tp.AccessToken == "" || tp.RefreshToken == "" {
		t.Fatal("token kosong")
	}
	// guest kedua dengan device sama -> token baru tapi player sama
	tp2, _ := svc.Guest(ctx, gameID, "device-1", "ua", "127.0.0.1")
	if tp.Player.ID != tp2.Player.ID {
		t.Errorf("player berbeda untuk device sama: %s vs %s", tp.Player.ID, tp2.Player.ID)
	}

	// refresh
	tp3, err := svc.Refresh(ctx, tp.RefreshToken, "ua", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if tp3.AccessToken == tp.AccessToken {
		t.Error("access token tidak berubah setelah refresh")
	}
	// reuse detection: pakai refresh lama -> harus gagal
	if _, err := svc.Refresh(ctx, tp.RefreshToken, "ua", "127.0.0.1"); err == nil {
		t.Error("reuse refresh token harus gagal")
	}
}

func TestRegisterLoginLink(t *testing.T) {
	svc, gameID, cleanup := setupIdentityTest(t)
	defer cleanup()
	ctx := context.Background()

	// guest dulu
	tp, _ := svc.Guest(ctx, gameID, "dev-2", "ua", "127.0.0.1")
	// link ke email
	if err := svc.Link(ctx, tp.Player.ID, gameID, "dina@example.com", "password123"); err != nil {
		t.Fatal(err)
	}
	// login dengan email
	tp2, err := svc.Login(ctx, gameID, "dina@example.com", "password123", "ua", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if tp2.Player.ID != tp.Player.ID {
		t.Errorf("player id berbeda setelah login: %s vs %s", tp2.Player.ID, tp.Player.ID)
	}
	// login salah password
	if _, err := svc.Login(ctx, gameID, "dina@example.com", "salah", "ua", "127.0.0.1"); err == nil {
		t.Error("login password salah harus gagal")
	}
	// register duplikat email
	if _, err := svc.Register(ctx, gameID, "dina@example.com", "password123", "ua", "127.0.0.1"); err == nil {
		t.Error("register email duplikat harus gagal")
	}
}

func TestBanEnforcement(t *testing.T) {
	svc, gameID, cleanup := setupIdentityTest(t)
	defer cleanup()
	ctx := context.Background()

	tp, _ := svc.Guest(ctx, gameID, "dev-3", "ua", "127.0.0.1")
	if err := svc.Ban(ctx, tp.Player.ID, "cheat"); err != nil {
		t.Fatal(err)
	}
	banned, _ := svc.IsBanned(ctx, tp.Player.ID)
	if !banned {
		t.Error("player harus banned")
	}
	// refresh token harus dicabut
	if _, err := svc.Refresh(ctx, tp.RefreshToken, "ua", "127.0.0.1"); err == nil {
		t.Error("refresh setelah ban harus gagal")
	}
	// unban
	if err := svc.Unban(ctx, tp.Player.ID); err != nil {
		t.Fatal(err)
	}
	banned2, _ := svc.IsBanned(ctx, tp.Player.ID)
	if banned2 {
		t.Error("player harus unban")
	}
}
