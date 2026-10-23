// Package app merangkai konfigurasi, DB, migrasi, server, dan lifecycle.
package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chava/raven/internal/backoffice"
	"github.com/chava/raven/internal/config"
	"github.com/chava/raven/internal/db"
	"log/slog"

	"github.com/chava/raven/internal/httpapi"
	"github.com/chava/raven/internal/identity"
	"github.com/chava/raven/internal/obs"
)

// App adalah konteks aplikasi yang sudah terinisialisasi.
type App struct {
	Config    *config.Config
	Logger    *slog.Logger
	Metrics   *obs.Metrics
	DB        *db.DB
	Server    *httpapi.Server
	RateLimit *httpapi.RateLimiter
}

// New membangun App: config, logger, metrics, DB, (opsional) migrasi, server.
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	log := obs.NewLogger(os.Stdout, cfg.LogLevel)
	metrics := obs.NewMetrics("dev")

	a := &App{Config: cfg, Logger: log, Metrics: metrics}

	dbConn, err := db.Open(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.WaitForDB)
	if err != nil {
		log.Error("gagal membuka database", "error", err)
		// Tetap lanjut tanpa DB? Untuk health endpoint berguna, tapi readiness gagal.
		// Kami tetap jalan supaya operator bisa diagnosis via /readyz.
		a.DB = nil
	} else {
		a.DB = dbConn
		if cfg.AutoMigrate {
			mig, err := db.NewDefaultMigrator(dbConn)
			if err != nil {
				dbConn.Close()
				return nil, fmt.Errorf("buat migrator: %w", err)
			}
			applied, err := mig.Up(ctx)
			if err != nil {
				dbConn.Close()
				return nil, fmt.Errorf("migrasi up: %w", err)
			}
			if len(applied) > 0 {
				log.Info("migrasi diterapkan", "versi", applied)
			}
		}
	}

	// Readiness selalu memeriksa database: bila DB nil (gagal konek), /readyz
	// harus 503 supaya operator tahu instance belum siap (PRD §6 ketersediaan).
	readiness := []httpapi.Check{
		{
			Name: "database",
			Fn: func(ctx context.Context) error {
				if a.DB == nil {
					return fmt.Errorf("database tidak terhubung")
				}
				return a.DB.Ping(ctx)
			},
		},
	}
	health := &httpapi.Health{
		Version:   "dev",
		StartedAt: time.Now(),
		Timeout:   2 * time.Second,
		Checks:    readiness,
	}

	routerOpts := httpapi.RouterOptions{
		Logger:         log,
		Metrics:        obs.NewRecorder(metrics),
		MaxBodyBytes:   cfg.MaxBodyBytes,
		TrustedProxies: cfg.TrustedProxies,
		CORSOrigins:    cfg.CORSOrigins,
	}
	rl := httpapi.NewRateLimiter(cfg.RateLimit.RPS, cfg.RateLimit.Burst)
	if cfg.RateLimit.Enabled {
		rl.StartGC()
	} else {
		// Rate limiter dinonaktifkan: burst sangat besar.
		rl = httpapi.NewRateLimiter(1e9, 1e9)
	}
	routerOpts.RateLimiter = rl

	router := httpapi.NewRouter(routerOpts)
	router.Get("/healthz", "health.liveness", health.LivenessHandler())
	router.Get("/readyz", "health.readiness", health.ReadinessHandler())
	router.Get("/metrics", "metrics", metrics.Handler().ServeHTTP)
	router.Get("/v1/meta", "meta", func(w http.ResponseWriter, r *http.Request) {
		httpapi.WriteJSON(w, 200, map[string]any{
			"name":       "raven",
			"version":    "dev",
			"api":        "v1",
			"started_at": health.StartedAt.UTC().Format(time.RFC3339),
		})
	})

	// --- Identity (M1) ---
	identityStore := identity.NewStore(nil)
	if a.DB != nil {
		identityStore = identity.NewStore(a.DB.Pool)
	}
	jwtIssuer := &identity.JWTIssuer{Secret: []byte(cfg.JWTSecret), TTL: cfg.AccessTokenTTL}
	identitySvc := &identity.Service{
		Store:       identityStore,
		JWT:         jwtIssuer,
		AccessTTL:   cfg.AccessTokenTTL,
		RefreshTTL:  cfg.RefreshTokenTTL,
		BanCacheTTL: 30 * time.Second,
	}
	identityHandler := identity.NewHandler(identitySvc)
	authMW := identity.NewAuthMiddleware(jwtIssuer, identitySvc)

	router.Post("/v1/auth/guest", "auth.guest", identityHandler.Guest)
	router.Post("/v1/auth/register", "auth.register", identityHandler.Register)
	router.Post("/v1/auth/login", "auth.login", identityHandler.Login)
	router.Post("/v1/auth/refresh", "auth.refresh", identityHandler.Refresh)
	router.Post("/v1/auth/logout", "auth.logout", identityHandler.Logout)
	router.Handle("POST", "/v1/auth/logout-all", "auth.logout_all", authMW.Handle(http.HandlerFunc(identityHandler.LogoutAll)))
	router.Handle("POST", "/v1/auth/link", "auth.link", authMW.Handle(http.HandlerFunc(identityHandler.Link)))

	adminBan := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.AdminToken == "" || r.Header.Get("X-Admin-Token") != cfg.AdminToken {
			httpapi.WriteProblem(w, r, httpapi.Forbidden("Admin token tidak valid."))
			return
		}
		var body struct {
			PlayerID string `json:"player_id"`
			Reason   string `json:"reason"`
			Ban      bool   `json:"ban"`
		}
		if prob := httpapi.DecodeJSON(r, 64*1024, &body); prob != nil {
			httpapi.WriteProblem(w, r, prob)
			return
		}
		if body.Ban {
			if err := identitySvc.Ban(r.Context(), body.PlayerID, body.Reason); err != nil {
				httpapi.WriteProblem(w, r, httpapi.ProblemFromError(err))
				return
			}
		} else {
			if err := identitySvc.Unban(r.Context(), body.PlayerID); err != nil {
				httpapi.WriteProblem(w, r, httpapi.ProblemFromError(err))
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
	router.Post("/v1/admin/players/ban", "admin.players.ban", adminBan)

	// --- Backoffice (M4) ---
	if a.DB != nil {
		backofficeSvc := backoffice.NewService(a.DB.Pool)
		backofficeHandler := backoffice.NewHandler(backofficeSvc)
		router.Post("/v1/admin/login", "admin.login", backofficeHandler.Login)
		router.Post("/v1/admin/users", "admin.users.create", func(w http.ResponseWriter, r *http.Request) {
			if cfg.AdminToken == "" || r.Header.Get("X-Admin-Token") != cfg.AdminToken {
				httpapi.WriteProblem(w, r, httpapi.Forbidden("Admin token tidak valid."))
				return
			}
			backofficeHandler.CreateAdmin(w, r)
		})
	}

	a.RateLimit = rl
	a.Server = &httpapi.Server{
		Config:    cfg,
		Logger:    log,
		Metrics:   metrics,
		Router:    router,
		Health:    health,
		RateLimit: rl,
	}
	return a, nil
}

// Run menjalankan server sampai ctx dibatalkan (SIGINT/SIGTERM).
func (a *App) Run(ctx context.Context) error {
	if a.Server == nil {
		return errors.New("server belum dibangun")
	}
	return a.Server.Run(ctx)
}

// Close menutup DB, rate limiter, dsb.
func (a *App) Close() {
	if a.RateLimit != nil {
		a.RateLimit.Stop()
	}
	if a.DB != nil {
		a.DB.Close()
	}
}

// SignalContext mengembalikan context yang dibatalkan pada SIGINT/SIGTERM.
func SignalContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-ch:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx
}
