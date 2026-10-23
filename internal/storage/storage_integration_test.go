//go:build integration

package storage

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/chava/raven/internal/db"
	"github.com/chava/raven/internal/identity"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupStorageTest(t *testing.T) (*Service, string, string, func()) {
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
	_, _ = pool.Exec(ctx, "delete from save_versions")
	_, _ = pool.Exec(ctx, "delete from saves")
	_, _ = pool.Exec(ctx, "delete from sessions")
	_, _ = pool.Exec(ctx, "delete from identities")
	_, _ = pool.Exec(ctx, "delete from players")
	_, _ = pool.Exec(ctx, "delete from games")

	var gameID string
	err = pool.QueryRow(ctx, `insert into games (name,slug,api_key_hash,api_key_prefix,secret_hash)
		values ('g','g','h','p','s') returning id`).Scan(&gameID)
	if err != nil {
		t.Fatal(err)
	}
	// buat player via identity store
	istore := identity.NewStore(pool)
	player, err := istore.CreatePlayer(ctx, gameID, "active", "tester")
	if err != nil {
		t.Fatal(err)
	}

	blobDir := t.TempDir()
	svc := NewService(pool, blobDir, 5)
	return svc, gameID, player.ID, func() { pool.Close() }
}

func TestSavePutAndGet(t *testing.T) {
	svc, gameID, playerID, cleanup := setupStorageTest(t)
	defer cleanup()
	ctx := context.Background()

	// PUT pertama
	res, err := svc.Put(ctx, PutInput{
		GameID: gameID, PlayerID: playerID, Slot: "main",
		Data: []byte(`{"level":1}`), ContentType: "application/json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Version != 1 {
		t.Errorf("version pertama = %d, mau 1", res.Version)
	}

	// GET
	got, err := svc.Get(ctx, gameID, playerID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data) != `{"level":1}` {
		t.Errorf("data = %s", string(got.Data))
	}
	if got.Version != 1 {
		t.Errorf("version get = %d", got.Version)
	}
	if got.Checksum == "" {
		t.Error("checksum kosong")
	}
}

func TestSaveOptimisticConcurrency(t *testing.T) {
	svc, gameID, playerID, cleanup := setupStorageTest(t)
	defer cleanup()
	ctx := context.Background()

	svc.Put(ctx, PutInput{GameID: gameID, PlayerID: playerID, Slot: "main", Data: []byte(`{"v":1}`)})

	// If-Match salah -> 409
	_, err := svc.Put(ctx, PutInput{GameID: gameID, PlayerID: playerID, Slot: "main", Data: []byte(`{"v":2}`), IfMatch: 99})
	if err == nil {
		t.Fatal("mau error 409, dapat nil")
	}
	pe, ok := err.(*httpProblem)
	if !ok || pe.status != 409 {
		t.Fatalf("mau 409 conflict, dapat %v", err)
	}

	// If-Match benar -> sukses version 2
	res, err := svc.Put(ctx, PutInput{GameID: gameID, PlayerID: playerID, Slot: "main", Data: []byte(`{"v":2}`), IfMatch: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Version != 2 {
		t.Errorf("version = %d, mau 2", res.Version)
	}
}

func TestSaveVersionHistory(t *testing.T) {
	svc, gameID, playerID, cleanup := setupStorageTest(t)
	defer cleanup()
	ctx := context.Background()

	for i := 1; i <= 7; i++ {
		data, _ := json.Marshal(map[string]int{"v": i})
		if i == 1 {
			svc.Put(ctx, PutInput{GameID: gameID, PlayerID: playerID, Slot: "main", Data: data})
		} else {
			svc.Put(ctx, PutInput{GameID: gameID, PlayerID: playerID, Slot: "main", Data: data, IfMatch: int64(i - 1)})
		}
	}
	// default keep 5 versi terakhir -> versions 3..7 (5 versi) tersisa
	versions, err := svc.ListVersions(ctx, gameID, playerID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 5 {
		t.Errorf("jumlah versi = %d, mau 5", len(versions))
	}
}

func TestSaveListSlots(t *testing.T) {
	svc, gameID, playerID, cleanup := setupStorageTest(t)
	defer cleanup()
	ctx := context.Background()

	svc.Put(ctx, PutInput{GameID: gameID, PlayerID: playerID, Slot: "a", Data: []byte(`1`)})
	svc.Put(ctx, PutInput{GameID: gameID, PlayerID: playerID, Slot: "b", Data: []byte(`2`)})

	slots, err := svc.ListSlots(ctx, gameID, playerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 2 {
		t.Errorf("jumlah slot = %d, mau 2", len(slots))
	}
}

func TestSaveDeltaSince(t *testing.T) {
	svc, gameID, playerID, cleanup := setupStorageTest(t)
	defer cleanup()
	ctx := context.Background()

	svc.Put(ctx, PutInput{GameID: gameID, PlayerID: playerID, Slot: "a", Data: []byte(`1`)})
	// ambil cursor setelah put pertama
	slots, _ := svc.ListSlots(ctx, gameID, playerID)
	cursor := slots[0].UpdatedAt

	svc.Put(ctx, PutInput{GameID: gameID, PlayerID: playerID, Slot: "b", Data: []byte(`2`)})

	delta, err := svc.Delta(ctx, gameID, playerID, cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(delta) != 1 {
		t.Errorf("delta = %d, mau 1", len(delta))
	}
	if len(delta) > 0 && delta[0].Slot != "b" {
		t.Errorf("delta slot = %s, mau b", delta[0].Slot)
	}
}

func TestSaveDeleteMarksTombstone(t *testing.T) {
	svc, gameID, playerID, cleanup := setupStorageTest(t)
	defer cleanup()
	ctx := context.Background()

	svc.Put(ctx, PutInput{GameID: gameID, PlayerID: playerID, Slot: "x", Data: []byte(`1`)})
	if err := svc.Delete(ctx, gameID, playerID, "x"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(ctx, gameID, playerID, "x")
	if err == nil {
		t.Fatal("mau error not found setelah delete")
	}
	_ = got
}
