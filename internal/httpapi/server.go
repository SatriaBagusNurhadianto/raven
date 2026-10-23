package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/chava/raven/internal/config"
)

// Server menggabungkan config, router, dan health endpoint menjadi satu handler.
type Server struct {
	Config    *config.Config
	Logger    *slog.Logger
	Metrics   any
	Router    *Router
	Health    *Health
	RateLimit *RateLimiter
}

// Handler menyusun router menjadi http.Handler final.
func (s *Server) Handler() http.Handler {
	if s.Router == nil {
		panic("httpapi: Server.Router nil")
	}
	return s.Router.Handler()
}

// Run menjalankan server dengan graceful shutdown.
func (s *Server) Run(ctx context.Context) error {
	if s.Config == nil {
		return errors.New("config nil")
	}
	srv := &http.Server{
		Addr:              s.Config.HTTPAddr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          nil,
	}
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", srv.Addr, err)
	}
	if s.Logger != nil {
		s.Logger.Info("raven mulai", "addr", ln.Addr().String(), "env", s.Config.Env)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.Config.ShutdownTimeout)
	defer cancel()
	if s.Logger != nil {
		s.Logger.Info("graceful shutdown", "timeout", s.Config.ShutdownTimeout.String())
	}
	return srv.Shutdown(shutdownCtx)
}
