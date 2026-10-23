package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const downMarker = "-- +migrate Down"

// Migration adalah satu file migrasi yang diparse.
type Migration struct {
	Version int
	Name    string
	Up      string
	Down    string
}

// LoadMigrations memparse seluruh file migrasi dari fs (untuk test).
func LoadMigrations(fsys fs.FS) ([]Migration, error) {
	if fsys == nil {
		fsys = migrationFS
	}
	entries, err := fs.Glob(fsys, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("glob migrasi: %w", err)
	}
	var out []Migration
	for _, e := range entries {
		raw, err := fs.ReadFile(fsys, e)
		if err != nil {
			return nil, fmt.Errorf("baca %s: %w", e, err)
		}
		base := path.Base(e)
		name := strings.TrimSuffix(base, ".sql")
		parts := strings.SplitN(name, "_", 2)
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("nama migrasi %q harus diawali angka versi: %w", base, err)
		}
		migName := ""
		if len(parts) > 1 {
			migName = parts[1]
		}
		body := string(raw)
		up := body
		down := ""
		if idx := strings.Index(body, downMarker); idx >= 0 {
			up = body[:idx]
			down = body[idx+len(downMarker):]
		}
		if strings.TrimSpace(down) == "" {
			return nil, fmt.Errorf("migrasi %s tidak punya blok Down (penanda %q)", base, downMarker)
		}
		out = append(out, Migration{
			Version: version,
			Name:    migName,
			Up:      strings.TrimSpace(up),
			Down:    strings.TrimSpace(down),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	for i := 1; i < len(out); i++ {
		if out[i].Version == out[i-1].Version {
			return nil, fmt.Errorf("versi migrasi duplikat: %d", out[i].Version)
		}
	}
	return out, nil
}

// DefaultMigrations memuat migrasi bawaan yang di-embed.
func DefaultMigrations() ([]Migration, error) {
	return LoadMigrations(migrationFS)
}

// Migrator mengeksekusi migrasi naik/turun terhadap database.
type Migrator struct {
	db         *DB
	migrations []Migration
}

// NewMigrator membuat migrator baru dari DB dan daftar migrasi.
func NewMigrator(db *DB, migrations []Migration) *Migrator {
	return &Migrator{db: db, migrations: migrations}
}

// NewDefaultMigrator membuat migrator dengan migrasi bawaan.
func NewDefaultMigrator(db *DB) (*Migrator, error) {
	migs, err := DefaultMigrations()
	if err != nil {
		return nil, err
	}
	return &Migrator{db: db, migrations: migs}, nil
}

const createMigrationsTable = `
create table if not exists schema_migrations (
	version bigint primary key,
	name text not null,
	applied_at timestamptz not null default now()
);
`

// ensureTable membuat tabel schema_migrations bila belum ada.
func (m *Migrator) ensureTable(ctx context.Context) error {
	_, err := m.db.Pool.Exec(ctx, createMigrationsTable)
	return err
}

// appliedVersions mengembalikan set versi yang sudah diterapkan.
func (m *Migrator) appliedVersions(ctx context.Context) (map[int]bool, error) {
	rows, err := m.db.Pool.Query(ctx, "select version from schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

// Up menerapkan semua migrasi yang belum diterapkan, dalam urutan versi.
func (m *Migrator) Up(ctx context.Context) ([]int, error) {
	if err := m.ensureTable(ctx); err != nil {
		return nil, fmt.Errorf("ensure table: %w", err)
	}
	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return nil, err
	}
	var done []int
	for _, mig := range m.migrations {
		if applied[mig.Version] {
			continue
		}
		if err := m.applyOne(ctx, mig, true); err != nil {
			return done, fmt.Errorf("migrasi %d (%s) up gagal: %w", mig.Version, mig.Name, err)
		}
		done = append(done, mig.Version)
	}
	return done, nil
}

// Down membalikkan satu migrasi terakhir yang diterapkan.
func (m *Migrator) Down(ctx context.Context) (int, error) {
	if err := m.ensureTable(ctx); err != nil {
		return 0, err
	}
	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return 0, err
	}
	// Cari versi tertinggi yang diterapkan.
	var target *Migration
	for i := range m.migrations {
		mig := &m.migrations[i]
		if applied[mig.Version] {
			if target == nil || mig.Version > target.Version {
				target = mig
			}
		}
	}
	if target == nil {
		return 0, nil
	}
	if err := m.applyOne(ctx, *target, false); err != nil {
		return 0, fmt.Errorf("migrasi %d (%s) down gagal: %w", target.Version, target.Name, err)
	}
	return target.Version, nil
}

// DownAll membalikkan semua migrasi yang diterapkan, dari tertinggi ke terendah.
func (m *Migrator) DownAll(ctx context.Context) ([]int, error) {
	if err := m.ensureTable(ctx); err != nil {
		return nil, err
	}
	var rolled []int
	for {
		v, err := m.Down(ctx)
		if err != nil {
			return rolled, err
		}
		if v == 0 {
			break
		}
		rolled = append(rolled, v)
	}
	return rolled, nil
}

// Status mengembalikan versi terakhir yang diterapkan (0 bila belum ada).
func (m *Migrator) Status(ctx context.Context) (int, error) {
	if err := m.ensureTable(ctx); err != nil {
		return 0, err
	}
	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return 0, err
	}
	max := 0
	for v := range applied {
		if v > max {
			max = v
		}
	}
	return max, nil
}

func (m *Migrator) applyOne(ctx context.Context, mig Migration, up bool) error {
	sql := mig.Up
	if !up {
		sql = mig.Down
	}
	tx, err := m.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Advisory lock per-tx supaya dua instance tidak berebut.
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock($1)", int64(7919)); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}

	// Re-cek status (race-safe) di dalam tx.
	var exists bool
	row := tx.QueryRow(ctx, "select exists(select 1 from schema_migrations where version=$1)", mig.Version)
	if err := row.Scan(&exists); err != nil {
		return fmt.Errorf("cek versi: %w", err)
	}
	if up && exists {
		return tx.Commit(ctx)
	}
	if !up && !exists {
		return tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("exec sql: %w", err)
	}
	if up {
		if _, err := tx.Exec(ctx, "insert into schema_migrations (version, name) values ($1,$2)", mig.Version, mig.Name); err != nil {
			return fmt.Errorf("record up: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, "delete from schema_migrations where version=$1", mig.Version); err != nil {
			return fmt.Errorf("record down: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// ErrNoMigration adalah sentinel untuk "tidak ada migrasi yang diterapkan".
var _ = pgx.ErrNoRows
