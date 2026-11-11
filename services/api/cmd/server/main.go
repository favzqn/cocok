package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cocok/cocok/services/api/internal/audit"
	"github.com/cocok/cocok/services/api/internal/auth"
	"github.com/cocok/cocok/services/api/internal/config"
	"github.com/cocok/cocok/services/api/internal/httpx"
	"github.com/cocok/cocok/services/api/internal/organizations"
	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/cocok/cocok/services/api/migrations"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(os.Getenv("LOG_LEVEL"))}))
	if err := run(log); err != nil {
		log.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := platform.OpenPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer platform.ClosePool(pool, log)

	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := platform.Migrate(ctx, pool, migrations.FS); err != nil {
			return err
		}
		log.Info("migrations applied")
		return nil
	}

	if err := platform.Migrate(ctx, pool, migrations.FS); err != nil {
		return err
	}

	queries := db.New(pool)
	authSvc := auth.NewService(queries, cfg.SessionTTL)
	orgSvc := organizations.NewService(queries, pool, audit.OrgAdapter{W: audit.NewWriter(queries, log)})

	router := httpx.NewRouter(httpx.Deps{
		Auth:      httpx.NewAuthHandlers(authSvc, log, cfg.SessionCookie),
		Orgs:      httpx.NewOrgHandlers(orgSvc, log),
		OrgsSvc:   orgSvc,
		Log:       log,
		WebOrigin: cfg.WebOrigin,
	})

	srv := &http.Server{
		Addr:              cfg.ServerAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", cfg.ServerAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutCtx)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
