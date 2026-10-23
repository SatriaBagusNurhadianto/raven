package ranking

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type httpProblem struct {
	status  int
	message string
}

func (e *httpProblem) Error() string { return e.message }

var ErrNotFound = errors.New("tidak ditemukan")

type Board struct {
	ID          string
	GameID      string
	Key         string
	Name        string
	Sort        string // asc/desc
	ResetPeriod string
	ScoreMode   string // best/latest/sum
	MinScore    *int64
	MaxScore    *int64
	MetaMaxBytes int
}

type ScoreEntry struct {
	PlayerID    string
	Score       float64
	BestScore   float64
	Submissions int64
	Meta        []byte
	UpdatedAt   time.Time
	Rank        int
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) CreateBoard(ctx context.Context, gameID, key, name, sort, resetPeriod, scoreMode string) (*Board, error) {
	if sort == "" {
		sort = "desc"
	}
	if resetPeriod == "" {
		resetPeriod = "none"
	}
	if scoreMode == "" {
		scoreMode = "best"
	}
	var id string
	err := s.pool.QueryRow(ctx, `
		insert into leaderboards (game_id, key, name, sort, reset_period, score_mode)
		values ($1::uuid, $2, $3, $4, $5, $6) returning id`,
		gameID, key, name, sort, resetPeriod, scoreMode).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &Board{ID: id, GameID: gameID, Key: key, Name: name, Sort: sort, ResetPeriod: resetPeriod, ScoreMode: scoreMode, MetaMaxBytes: 512}, nil
}

func (s *Service) GetBoard(ctx context.Context, gameID, key string) (*Board, error) {
	var b Board
	var minScore, maxScore *int64
	err := s.pool.QueryRow(ctx, `
		select id, sort, reset_period, score_mode, min_score, max_score, meta_max_bytes
		from leaderboards where game_id=$1::uuid and key=$2`, gameID, key).
		Scan(&b.ID, &b.Sort, &b.ResetPeriod, &b.ScoreMode, &minScore, &maxScore, &b.MetaMaxBytes)
	if err != nil {
		return nil, &httpProblem{status: 404, message: "board tidak ditemukan."}
	}
	b.GameID = gameID
	b.Key = key
	b.MinScore = minScore
	b.MaxScore = maxScore
	return &b, nil
}

func periodKey(period string, now time.Time) string {
	switch period {
	case "daily":
		return now.UTC().Format("2006-01-02")
	case "weekly":
		y, w := now.UTC().ISOWeek()
		return time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, (w-1)*7).Format("2006-01-02")
	case "monthly":
		return now.UTC().Format("2006-01")
	default:
		return ""
	}
}

func (s *Service) SubmitScore(ctx context.Context, gameID, boardKey, playerID string, score float64, meta []byte, now time.Time) (*ScoreEntry, error) {
	board, err := s.GetBoard(ctx, gameID, boardKey)
	if err != nil {
		return nil, err
	}
	if board.MinScore != nil && score < float64(*board.MinScore) {
		return nil, &httpProblem{status: 400, message: "skor di bawah min."}
	}
	if board.MaxScore != nil && score > float64(*board.MaxScore) {
		return nil, &httpProblem{status: 400, message: "skor di atas max."}
	}
	pk := periodKey(board.ResetPeriod, now)

	// upsert
	var bestScore float64
	var submissions int64
	err = s.pool.QueryRow(ctx, `
		select best_score, submissions from scores
		where leaderboard_id=$1::uuid and period_key=$2 and player_id=$3::uuid`,
		board.ID, pk, playerID).Scan(&bestScore, &submissions)
	exists := err == nil
	if err != nil && err.Error() != "no rows in result set" {
		// pgx ErrNoRows
	}

	newBest := score
	newScore := score
	switch board.ScoreMode {
	case "best":
		if exists && bestScore > score && board.Sort == "desc" {
			newBest = bestScore
		}
		if exists && bestScore < score && board.Sort == "asc" {
			newBest = bestScore
		}
		newScore = newBest
	case "latest":
		newScore = score
		newBest = score
	case "sum":
		if exists {
			newScore = score + bestScore
		}
		newBest = newScore
	}
	if !exists {
		bestScore = score
		submissions = 0
	}
	submissions++

	_, err = s.pool.Exec(ctx, `
		insert into scores (leaderboard_id, period_key, player_id, score, best_score, submissions, meta, updated_at)
		values ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, now())
		on conflict (leaderboard_id, period_key, player_id) do update set
			score = excluded.score,
			best_score = excluded.best_score,
			submissions = scores.submissions + 1,
			meta = excluded.meta,
			updated_at = now()`,
		board.ID, pk, playerID, newScore, newBest, 1, meta)
	if err != nil {
		return nil, err
	}
	// fix submissions count properly
	_, _ = s.pool.Exec(ctx, `
		update scores set submissions=$1 where leaderboard_id=$2::uuid and period_key=$3 and player_id=$4::uuid`,
		submissions, board.ID, pk, playerID)

	return &ScoreEntry{PlayerID: playerID, Score: newScore, BestScore: newBest, Submissions: submissions, Meta: meta, UpdatedAt: time.Now()}, nil
}

func (s *Service) GetTop(ctx context.Context, gameID, boardKey string, limit int, periodKey string, now time.Time) ([]ScoreEntry, error) {
	board, err := s.GetBoard(ctx, gameID, boardKey)
	if err != nil {
		return nil, err
	}
	if periodKey == "" {
		periodKey = periodKeyStr(board.ResetPeriod, now)
	}
	order := "desc"
	if board.Sort == "asc" {
		order = "asc"
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		select player_id, score, best_score, submissions, meta, updated_at
		from scores
		where leaderboard_id=$1::uuid and period_key=$2
		order by score `+order+`, updated_at asc
		limit $3`, board.ID, periodKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScoreEntry
	rank := 0
	for rows.Next() {
		var e ScoreEntry
		var meta []byte
		if err := rows.Scan(&e.PlayerID, &e.Score, &e.BestScore, &e.Submissions, &meta, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Meta = meta
		rank++
		e.Rank = rank
		out = append(out, e)
	}
	return out, rows.Err()
}

func periodKeyStr(period string, now time.Time) string { return periodKey(period, now) }

func (s *Service) GetAroundMe(ctx context.Context, gameID, boardKey, playerID string, rng int, now time.Time) ([]ScoreEntry, error) {
	board, err := s.GetBoard(ctx, gameID, boardKey)
	if err != nil {
		return nil, err
	}
	pk := periodKey(board.ResetPeriod, now)
	order := "desc"
	if board.Sort == "asc" {
		order = "asc"
	}
	// find player's score
	var playerScore float64
	var playerUpdated time.Time
	err = s.pool.QueryRow(ctx, `
		select score, updated_at from scores
		where leaderboard_id=$1::uuid and period_key=$2 and player_id=$3::uuid`,
		board.ID, pk, playerID).Scan(&playerScore, &playerUpdated)
	if err != nil {
		return nil, &httpProblem{status: 404, message: "pemain belum ada skor."}
	}
	// count better players
	var better int
	if board.Sort == "desc" {
		_ = s.pool.QueryRow(ctx, `
			select count(*) from scores
			where leaderboard_id=$1::uuid and period_key=$2
			  and (score > $3 or (score = $3 and updated_at < $4))`,
			board.ID, pk, playerScore, playerUpdated).Scan(&better)
	} else {
		_ = s.pool.QueryRow(ctx, `
			select count(*) from scores
			where leaderboard_id=$1::uuid and period_key=$2
			  and (score < $3 or (score = $3 and updated_at < $4))`,
			board.ID, pk, playerScore, playerUpdated).Scan(&better)
	}
	startRank := better - rng
	if startRank < 0 {
		startRank = 0
	}
	limit := rng*2 + 1
	rows, err := s.pool.Query(ctx, `
		select player_id, score, best_score, submissions, meta, updated_at
		from scores
		where leaderboard_id=$1::uuid and period_key=$2
		order by score `+order+`, updated_at asc
		offset $3 limit $4`, board.ID, pk, startRank, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScoreEntry
	rank := startRank + 1
	for rows.Next() {
		var e ScoreEntry
		var meta []byte
		if err := rows.Scan(&e.PlayerID, &e.Score, &e.BestScore, &e.Submissions, &meta, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Meta = meta
		e.Rank = rank
		rank++
		out = append(out, e)
	}
	return out, rows.Err()
}
