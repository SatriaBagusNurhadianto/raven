//go:build integration

package db

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrateUpDownIntegration(t *testing.T) {
	dsn := os.Getenv("RAVEN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("RAVEN_TEST_DATABASE_URL tidak diisi; lewati integration test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("koneksi gagal: %v", err)
	}
	defer pool.Close()

	dbConn := &DB{Pool: pool}
	mig, err := NewDefaultMigrator(dbConn)
	if err != nil {
		t.Fatalf("migrator: %v", err)
	}

	// Bersihkan state dari run sebelumnya.
	if _, err := mig.DownAll(ctx); err != nil {
		t.Fatalf("DownAll awal gagal: %v", err)
	}

	// Up pertama.
	applied, err := mig.Up(ctx)
	if err != nil {
		t.Fatalf("Up pertama gagal: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("mau 1 migrasi diterapkan, dapat %d: %v", len(applied), applied)
	}

	// Up kedua harus idempoten.
	applied2, err := mig.Up(ctx)
	if err != nil {
		t.Fatalf("Up kedua gagal: %v", err)
	}
	if len(applied2) != 0 {
		t.Fatalf("Up kedua harus idempoten, dapat %v", applied2)
	}

	// Verifikasi 11 tabel inti ada.
	tables := []string{"games", "players", "identities", "sessions", "saves",
		"save_versions", "leaderboards", "scores", "admin_users", "audit_logs",
		"instance_settings", "schema_migrations"}
	for _, tbl := range tables {
		var exists bool
		err := pool.QueryRow(ctx,
			"select exists(select 1 from information_schema.tables where table_schema='public' and table_name=$1)",
			tbl).Scan(&exists)
		if err != nil || !exists {
			t.Errorf("tabel %s tidak ada (err=%v)", tbl, err)
		}
	}

	// Verifikasi trigger audit immutable: UPDATE harus gagal.
	_, err = pool.Exec(ctx, "insert into audit_logs (actor_type, action) values ('system','test')")
	if err != nil {
		t.Fatalf("insert audit gagal: %v", err)
	}
	_, err = pool.Exec(ctx, "update audit_logs set action='x' where action='test'")
	if err == nil {
		t.Error("UPDATE audit_logs harus gagal (immutable), tapi lolos")
	}
	_, err = pool.Exec(ctx, "delete from audit_logs where action='test'")
	if err == nil {
		t.Error("DELETE audit_logs harus gagal (immutable), tapi lolos")
	}

	// Verifikasi instance_settings default.
	var mode string
	if err := pool.QueryRow(ctx, "select value #>> '{}' from instance_settings where key='identity_mode'").Scan(&mode); err != nil {
		t.Fatalf("baca identity_mode gagal: %v", err)
	}
	if mode != "per_game" {
		t.Errorf("identity_mode = %q, mau per_game", mode)
	}

	// Down: rollback dan verifikasi tabel hilang.
	rolled, err := mig.DownAll(ctx)
	if err != nil {
		t.Fatalf("DownAll gagal: %v", err)
	}
	if len(rolled) != 1 {
		t.Fatalf("mau 1 migrasi dibalik, dapat %d", len(rolled))
	}
	var gamesExists bool
	err = pool.QueryRow(ctx,
		"select exists(select 1 from information_schema.tables where table_schema='public' and table_name='games')").
		Scan(&gamesExists)
	if err != nil || gamesExists {
		t.Errorf("tabel games masih ada setelah down (err=%v)", err)
	}

	// Kembalikan ke state up agar DB siap pakai.
	if _, err := mig.Up(ctx); err != nil {
		t.Fatalf("Up ulang gagal: %v", err)
	}
}
