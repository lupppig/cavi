// Command api runs the CAVI HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/config"
	"github.com/lupppig/cavi/apps/api/internal/database"
	"github.com/lupppig/cavi/apps/api/internal/httpapi"
	"github.com/lupppig/cavi/apps/api/internal/identity"
	"github.com/lupppig/cavi/apps/api/internal/session"
	"github.com/lupppig/cavi/apps/api/internal/waitlist"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg.Env)
	slog.SetDefault(logger)

	db := database.Open(cfg.DatabaseURL)
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var auth *httpapi.AuthOptions
	if cfg.AuthEnabled() {
		sessions := session.NewStore(db, cfg.SessionTTL)
		go sweepExpiredSessions(ctx, logger, sessions)
		auth = &httpapi.AuthOptions{
			Provider: identity.NewWorkOS(
				cfg.WorkOSAPIKey,
				cfg.WorkOSClientID,
				cfg.PublicAPIURL+"/api/v1/auth/callback",
				cfg.WebURL+"/",
			),
			Accounts:      account.NewService(db),
			Sessions:      sessions,
			WebURL:        cfg.WebURL,
			SecureCookies: cfg.SecureCookies(),
		}
	} else {
		logger.Warn("sign-in disabled: WORKOS_API_KEY and WORKOS_CLIENT_ID are not set")
	}

	router, err := httpapi.NewRouter(httpapi.Options{
		Logger:         logger,
		DB:             db,
		Waitlist:       waitlist.NewService(waitlist.NewPostgresStore(db)),
		Auth:           auth,
		AllowedOrigins: cfg.AllowedOrigins,
		TrustedProxies: cfg.TrustedProxies,
		Debug:          cfg.Env == config.EnvDevelopment,
	})
	if err != nil {
		return fmt.Errorf("build router: %w", err)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("api listening", slog.String("addr", cfg.HTTPAddr), slog.String("env", cfg.Env))
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	logger.Info("shutting down", slog.Duration("timeout", cfg.ShutdownTimeout))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("shutdown complete")
	return nil
}

// sweepExpiredSessions deletes expired sessions hourly until ctx ends.
// Expired sessions are already rejected on lookup; this only reclaims rows.
func sweepExpiredSessions(ctx context.Context, logger *slog.Logger, store *session.Store) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := store.DeleteExpired(ctx)
			if err != nil {
				logger.Warn("expired session sweep failed", slog.String("error", err.Error()))
				continue
			}
			if n > 0 {
				logger.Info("expired sessions removed", slog.Int64("count", n))
			}
		}
	}
}

func newLogger(env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if env == config.EnvDevelopment {
		opts.Level = slog.LevelDebug
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
