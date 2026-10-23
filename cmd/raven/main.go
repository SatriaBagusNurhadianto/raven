package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/chava/raven/internal/app"
	"github.com/chava/raven/internal/config"
	"github.com/chava/raven/internal/db"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		os.Args = append(os.Args, "serve")
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "serve":
		err = cmdServe(args)
	case "migrate":
		err = cmdMigrate(args)
	case "doctor":
		err = cmdDoctor(args)
	case "backup":
		err = cmdBackup(args)
	case "version":
		fmt.Printf("raven %s (commit %s, built %s)\n", version, commit, date)
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		err = fmt.Errorf("perintah tidak dikenal: %s", cmd)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "raven: %v\n", err)
		os.Exit(1)
	}
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configFile := fs.String("config", "", "path file .env (opsional, belum dipakai)")
	_ = configFile
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("konfigurasi: %w", err)
	}
	a, err := app.New(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer a.Close()

	ctx := app.SignalContext()
	return a.Run(ctx)
}

func cmdMigrate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("subperintah migrate wajib: up | down | status")
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("konfigurasi: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dbConn, err := db.Open(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.WaitForDB)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer dbConn.Close()

	mig, err := db.NewDefaultMigrator(dbConn)
	if err != nil {
		return fmt.Errorf("migrator: %w", err)
	}
	switch args[0] {
	case "up":
		applied, err := mig.Up(ctx)
		if err != nil {
			return err
		}
		if len(applied) == 0 {
			fmt.Println("migrasi: tidak ada yang baru")
		} else {
			fmt.Printf("migrasi diterapkan: %v\n", applied)
		}
	case "down":
		v, err := mig.Down(ctx)
		if err != nil {
			return err
		}
		if v == 0 {
			fmt.Println("migrasi: tidak ada yang bisa dibalik")
		} else {
			fmt.Printf("migrasi %d dibalik\n", v)
		}
	case "status":
		v, err := mig.Status(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("versi migrasi terakhir: %d\n", v)
	default:
		return fmt.Errorf("subperintah migrate tidak dikenal: %s", args[0])
	}
	return nil
}

func cmdDoctor(args []string) error {
	cfg, err := config.FromEnv()
	checks := []struct {
		name   string
		ok     bool
		detail string
	}{}
	add := func(name string, ok bool, detail string) {
		checks = append(checks, struct {
			name   string
			ok     bool
			detail string
		}{name, ok, detail})
	}

	if err != nil {
		add("config", false, err.Error())
	} else {
		add("config", true, "valid ("+cfg.Env+")")
		// TLS
		tlsOK := cfg.IsProduction() == false || cfg.TLS.TerminatedByProxy || cfg.TLS.AllowInsecure
		add("tls", tlsOK, map[bool]string{true: "ok", false: "produksi tanpa TLS terkonfirmasi"}[tlsOK])
		// JWT secret
		secretOK := cfg.JWTSecret != config.DefaultJWTSecret && len(cfg.JWTSecret) >= 32
		if !cfg.IsProduction() {
			secretOK = true
		}
		add("jwt_secret", secretOK, map[bool]string{true: "ok", false: "secret lemah/default"}[secretOK])
		// Rate limit
		add("rate_limit", cfg.RateLimit.Enabled, map[bool]string{true: "aktif", false: "nonaktif"}[cfg.RateLimit.Enabled])
		// Blob dir
		if err := os.MkdirAll(cfg.BlobDir, 0o750); err != nil {
			add("blob_dir", false, err.Error())
		} else {
			tmp := cfg.BlobDir + "/.doctor"
			if err := os.WriteFile(tmp, []byte("ok"), 0o600); err != nil {
				add("blob_dir", false, err.Error())
			} else {
				_ = os.Remove(tmp)
				add("blob_dir", true, cfg.BlobDir)
			}
		}
		// DB
		if cfg.DatabaseURL != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			dbConn, derr := db.Open(ctx, cfg.DatabaseURL, cfg.DBMaxConns, 5*time.Second)
			if derr != nil {
				add("database", false, derr.Error())
			} else {
				v, _ := db.NewDefaultMigrator(dbConn)
				status, _ := v.Status(ctx)
				dbConn.Close()
				add("database", true, fmt.Sprintf("terhubung, migrasi v%d", status))
			}
		}
	}

	failed := 0
	for _, c := range checks {
		mark := "OK"
		if !c.ok {
			mark = "GAGAL"
			failed++
		}
		fmt.Printf("[%-6s] %-14s %s\n", mark, c.name, c.detail)
	}
	if failed > 0 {
		return fmt.Errorf("%d pemeriksaan gagal", failed)
	}
	return nil
}

func usage() {
	fmt.Println(`raven — self-hosted game backend

Penggunaan:
  raven serve            Jalankan server HTTP (default)
  raven migrate up       Terapkan migrasi yang belum diterapkan
  raven migrate down     Balikkan satu migrasi terakhir
  raven migrate status   Tampilkan versi migrasi terakhir
  raven doctor           Periksa konfigurasi dan koneksi
  raven version          Tampilkan versi

Environment variable: lihat deploy/.env.example`)
}
