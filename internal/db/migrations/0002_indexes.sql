-- 0002_indexes: indeks tambahan untuk performa query per-player.
-- +migrate Up
create index if not exists scores_player_idx on scores (player_id, leaderboard_id);
-- +migrate Down
drop index if exists scores_player_idx;