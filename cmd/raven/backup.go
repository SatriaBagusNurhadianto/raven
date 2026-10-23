package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/chava/raven/internal/config"
	"github.com/chava/raven/internal/db"
)

// Backup creates a JSON dump of all core tables.
func cmdBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	out := fs.String("out", "raven-backup.json", "output file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	dbConn, err := db.Open(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.WaitForDB)
	if err != nil {
		return err
	}
	defer dbConn.Close()

	tables := []string{"games", "players", "identities", "sessions", "saves", "save_versions", "leaderboards", "scores", "admin_users", "audit_logs", "instance_settings"}
	dump := map[string]any{}
	for _, t := range tables {
		rows, err := dbConn.Pool.Query(ctx, "select * from "+t)
		if err != nil {
			return fmt.Errorf("query %s: %w", t, err)
		}
		cols := rows.FieldDescriptions()
		colNames := make([]string, len(cols))
		for i, c := range cols {
			colNames[i] = string(c.Name)
		}
		var records []map[string]any
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				rows.Close()
				return err
			}
			rec := map[string]any{}
			for i, v := range vals {
				rec[colNames[i]] = v
			}
			records = append(records, rec)
		}
		rows.Close()
		dump[t] = records
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(dump); err != nil {
		return err
	}
	fmt.Printf("backup written: %s\n", *out)
	return nil
}
