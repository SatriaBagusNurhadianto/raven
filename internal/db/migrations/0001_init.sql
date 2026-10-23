-- 0001_init: skema inti Raven (PRD §7.3) + tabel pendukung FR-S3/FR-T2.

-- +migrate Up

-- Fungsi pemicu updated_at.
create or replace function raven_touch_updated_at()
returns trigger as $$
begin
	new.updated_at = now();
	return new;
end;
$$ language plpgsql;

-- Game (tenant) + kredensial API.
create table games (
	id uuid primary key default gen_random_uuid(),
	name text not null,
	slug text not null unique,
	api_key_hash text not null unique,
	api_key_prefix text not null,
	secret_hash text not null,
	settings jsonb not null default '{}'::jsonb,
	disabled_at timestamptz,
	created_at timestamptz not null default now(),
	updated_at timestamptz not null default now()
);
create trigger games_touch_updated_at
before update on games
for each row execute function raven_touch_updated_at();

-- Pemain. MVP: terisolasi per game (game_id NOT NULL). Identitas lintas-game
-- disiapkan sebagai opsi instance, belum diimplementasi (PRD §15.1).
create table players (
	id uuid primary key default gen_random_uuid(),
	game_id uuid not null references games(id) on delete cascade,
	status text not null default 'active'
		check (status in ('active','banned','deleted')),
	display_name text,
	banned_at timestamptz,
	ban_reason text,
	created_at timestamptz not null default now(),
	last_seen timestamptz not null default now()
);
create index players_game_created_idx on players (game_id, created_at desc);
create index players_game_last_seen_idx on players (game_id, last_seen desc);
create index players_game_status_idx on players (game_id, status);

-- Identitas (guest device, email, OAuth). external_id unik per game+provider.
create table identities (
	id uuid primary key default gen_random_uuid(),
	player_id uuid not null references players(id) on delete cascade,
	game_id uuid not null references games(id) on delete cascade,
	provider text not null
		check (provider in ('guest','email','google','discord','steam')),
	external_id text not null,
	password_hash text,
	email_verified_at timestamptz,
	created_at timestamptz not null default now(),
	unique (game_id, provider, external_id)
);
create index identities_player_idx on identities (player_id);

-- Sesi refresh token (rotasi + deteksi reuse, FR-A4/A5).
create table sessions (
	id uuid primary key default gen_random_uuid(),
	player_id uuid not null references players(id) on delete cascade,
	game_id uuid not null references games(id) on delete cascade,
	family_id uuid not null,
	refresh_hash text not null unique,
	device_id text,
	user_agent text,
	ip inet,
	expires_at timestamptz not null,
	revoked_at timestamptz,
	revoked_reason text,
	rotated_to uuid references sessions(id) on delete set null,
	created_at timestamptz not null default now(),
	last_used_at timestamptz
);
create index sessions_player_idx on sessions (player_id, expires_at);
create index sessions_family_idx on sessions (family_id);
create index sessions_expires_idx on sessions (expires_at) where revoked_at is null;

-- Cloud save: satu baris = slot terkini; versioning di save_versions (FR-S3).
create table saves (
	id uuid primary key default gen_random_uuid(),
	player_id uuid not null references players(id) on delete cascade,
	game_id uuid not null references games(id) on delete cascade,
	slot text not null,
	version bigint not null default 1,
	blob_ref text not null,
	checksum text not null,
	size_bytes bigint not null check (size_bytes >= 0),
	content_type text,
	encoding text not null default 'identity'
		check (encoding in ('identity','gzip','zstd')),
	is_deleted boolean not null default false,
	updated_at timestamptz not null default now(),
	created_at timestamptz not null default now(),
	unique (game_id, player_id, slot)
);
create index saves_player_updated_idx on saves (game_id, player_id, updated_at desc);
create trigger saves_touch_updated_at
before update on saves
for each row execute function raven_touch_updated_at();

create table save_versions (
	id bigserial primary key,
	save_id uuid not null references saves(id) on delete cascade,
	version bigint not null,
	blob_ref text not null,
	checksum text not null,
	size_bytes bigint not null check (size_bytes >= 0),
	encoding text not null default 'identity'
		check (encoding in ('identity','gzip','zstd')),
	created_at timestamptz not null default now(),
	unique (save_id, version)
);

-- Leaderboard.
create table leaderboards (
	id uuid primary key default gen_random_uuid(),
	game_id uuid not null references games(id) on delete cascade,
	key text not null,
	name text not null,
	sort text not null default 'desc' check (sort in ('asc','desc')),
	reset_period text not null default 'none' check (reset_period in ('none','daily','weekly','monthly')),
	score_mode text not null default 'best' check (score_mode in ('best','latest','sum')),
	min_score bigint,
	max_score bigint,
	meta_max_bytes int not null default 512,
	config jsonb not null default '{}'::jsonb,
	created_at timestamptz not null default now(),
	updated_at timestamptz not null default now(),
	unique (game_id, key)
);
create trigger leaderboards_touch_updated_at
before update on leaderboards
for each row execute function raven_touch_updated_at();

-- Skor per periode. period_key = '' untuk all-time, '2026-10-22' harian, dsb.
create table scores (
	leaderboard_id uuid not null references leaderboards(id) on delete cascade,
	period_key text not null,
	player_id uuid not null references players(id) on delete cascade,
	score numeric(30,6) not null,
	best_score numeric(30,6) not null,
	submissions bigint not null default 1,
	meta jsonb,
	updated_at timestamptz not null default now(),
	primary key (leaderboard_id, period_key, player_id)
);
create index scores_rank_idx on scores (leaderboard_id, period_key, score desc, updated_at asc);

-- Admin dashboard (FR-D1).
create table admin_users (
	id uuid primary key default gen_random_uuid(),
	email text not null unique,
	password_hash text not null,
	role text not null default 'viewer' check (role in ('owner','admin','viewer')),
	totp_secret text,
	totp_enabled boolean not null default false,
	last_login_at timestamptz,
	disabled_at timestamptz,
	created_at timestamptz not null default now()
);

-- Audit log immutable (FR-D4).
create table audit_logs (
	id bigserial primary key,
	actor_type text not null check (actor_type in ('admin','player','system')),
	actor_id text,
	action text not null,
	target_type text,
	target_id text,
	game_id uuid references games(id) on delete set null,
	request_id text,
	ip inet,
	metadata jsonb not null default '{}'::jsonb,
	created_at timestamptz not null default now()
);
create index audit_logs_created_idx on audit_logs (created_at desc);
create index audit_logs_actor_idx on audit_logs (actor_type, actor_id, created_at desc);
create index audit_logs_game_idx on audit_logs (game_id, created_at desc);

create or replace function raven_audit_logs_immutable()
returns trigger as $$
begin
	raise exception 'audit_logs bersifat immutable: %s tidak diizinkan', tg_op;
end;
$$ language plpgsql;

create trigger audit_logs_no_update
before update on audit_logs
for each row execute function raven_audit_logs_immutable();

create trigger audit_logs_no_delete
before delete on audit_logs
for each row execute function raven_audit_logs_immutable();

-- Pengaturan instance (opsi lintas-game, dsb.).
create table instance_settings (
	key text primary key,
	value jsonb not null,
	updated_at timestamptz not null default now()
);
insert into instance_settings (key, value)
values ('identity_mode', '"per_game"'::jsonb)
on conflict (key) do nothing;

-- +migrate Down

drop trigger if exists audit_logs_no_delete on audit_logs;
drop trigger if exists audit_logs_no_update on audit_logs;
drop function if exists raven_audit_logs_immutable();
drop table if exists instance_settings;
drop table if exists audit_logs;
drop table if exists admin_users;
drop table if exists scores;
drop table if exists leaderboards;
drop table if exists save_versions;
drop table if exists saves;
drop table if exists sessions;
drop table if exists identities;
drop table if exists players;
drop table if exists games;
drop function if exists raven_touch_updated_at();
