//go:build integration

package ranking

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/chava/raven/internal/db"
	"github.com/chava/raven/internal/identity"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupRankingTest(t *testing.T) (*Service, string, string, string, func()) {
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
	_, _ = pool.Exec(ctx, "delete from scores")
	_, _ = pool.Exec(ctx, "delete from leaderboards")
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
	istore := identity.NewStore(pool)
	p1, _ := istore.CreatePlayer(ctx, gameID, "active", "p1")
	p2, _ := istore.CreatePlayer(ctx, gameID, "active", "p2")

	svc := NewService(pool)
	return svc, gameID, p1.ID, p2.ID, func() { pool.Close() }
}

func TestRankingSubmitAndGetTop(t *testing.T) {
	svc, gameID, p1, p2, cleanup := setupRankingTest(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now()

	board, err := svc.CreateBoard(ctx, gameID, "kills", "Kills", "desc", "none", "best")
	if err != nil {
		t.Fatal(err)
	}
	if board.ID == "" {
		t.Fatal("no board id")
	}

	_, err = svc.SubmitScore(ctx, gameID, "kills", p1, 10, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SubmitScore(ctx, gameID, "kills", p2, 25, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	// best mode: p1 submit 5 (lower) keeps 10
	_, err = svc.SubmitScore(ctx, gameID, "kills", p1, 5, nil, now)
	if err != nil {
		t.Fatal(err)
	}

	top, err := svc.GetTop(ctx, gameID, "kills", 10, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 2 {
		t.Fatalf("top len = %d", len(top))
	}
	if top[0].PlayerID != p2 || top[0].Score != 25 {
		t.Errorf("rank1 = %+v", top[0])
	}
	if top[1].PlayerID != p1 || top[1].Score != 10 {
		t.Errorf("rank2 = %+v", top[1])
	}
}
