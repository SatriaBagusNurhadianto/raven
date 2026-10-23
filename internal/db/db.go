// Package db menyediakan koneksi Postgres (pgxpool) dan runner migrasi.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB membungkus pgxpool.Pool agar konfigurasi dan lifecycle terpusat.
type DB struct {
	Pool   *pgxpool.Pool
	Logger interface{ Debug(string, ...any) }
}

// Open membuka pool koneksi dengan retry sampai siap atau timeout.
func Open(ctx context.Context, dsn string, maxConns int32, waitTimeout time.Duration) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	if cfg.MinConns == 0 {
		cfg.MinConns = 1
	}
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	cfg.ConnConfig.RuntimeParams["application_name"] = "raven"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("buat pool: %w", err)
	}

	if waitTimeout <= 0 {
		waitTimeout = 30 * time.Second
	}
	deadline := time.Now().Add(waitTimeout)
	var lastErr error
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		lastErr = pool.Ping(pingCtx)
		cancel()
		if lastErr == nil {
			return &DB{Pool: pool}, nil
		}
		if time.Now().After(deadline) {
			pool.Close()
			return nil, fmt.Errorf("database tidak siap setelah %s: %w", waitTimeout, lastErr)
		}
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// Ping memeriksa koneksi ke database.
func (d *DB) Ping(ctx context.Context) error {
	if d == nil || d.Pool == nil {
		return fmt.Errorf("database belum terinisialisasi")
	}
	return d.Pool.Ping(ctx)
}

// Close menutup pool.
func (d *DB) Close() {
	if d != nil && d.Pool != nil {
		d.Pool.Close()
	}
}
