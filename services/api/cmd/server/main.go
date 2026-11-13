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

	"github.com/cocok/cocok/services/api/internal/accounts"
	"github.com/cocok/cocok/services/api/internal/audit"
	"github.com/cocok/cocok/services/api/internal/auth"
	"github.com/cocok/cocok/services/api/internal/config"
	"github.com/cocok/cocok/services/api/internal/exceptions"
	"github.com/cocok/cocok/services/api/internal/httpx"
	"github.com/cocok/cocok/services/api/internal/imports"
	"github.com/cocok/cocok/services/api/internal/integrations"
	"github.com/cocok/cocok/services/api/internal/notifications"
	"github.com/cocok/cocok/services/api/internal/organizations"
	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/cocok/cocok/services/api/internal/reconciliation"
	"github.com/cocok/cocok/services/api/internal/reports"
	"github.com/cocok/cocok/services/api/internal/settlements"
	"github.com/cocok/cocok/services/api/internal/webhooks"
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
	auditWriter := audit.NewWriter(queries, log)
	authSvc := auth.NewService(queries, cfg.SessionTTL, notifications.NewOutboxMailer(queries, log), cfg.WebOrigin)
	orgSvc := organizations.NewService(queries, pool, audit.OrgAdapter{W: auditWriter})
	acctSvc := accounts.NewService(queries, pool, auditWriter)
	impSvc := imports.NewService(queries, pool, auditWriter)
	excSvc := exceptions.NewService(queries, pool, auditWriter)
	reconSvc := reconciliation.NewService(queries, pool, auditWriter, excSvc)
	setlSvc := settlements.NewService(queries, pool, auditWriter)
	repSvc := reports.NewService(queries)
	intgSvc := integrations.NewService(queries, pool, auditWriter, cfg.WebOrigin)
	whkSvc := webhooks.NewService(queries, pool, auditWriter)

	router := httpx.NewRouter(httpx.Deps{
		Auth:         httpx.NewAuthHandlers(authSvc, log, cfg.SessionCookie),
		Orgs:         httpx.NewOrgHandlers(orgSvc, log),
		Accounts:     httpx.NewAccountHandlers(acctSvc, log),
		Imports:      httpx.NewImportHandlers(impSvc, log),
		Recon:        httpx.NewReconHandlers(reconSvc, log),
		Exceptions:   httpx.NewExceptionHandlers(excSvc, log),
		Settlements:  httpx.NewSettlementHandlers(setlSvc, log),
		Reports:      httpx.NewReportHandlers(repSvc, log),
		Integrations: httpx.NewIntegrationHandlers(intgSvc, log),
		Webhooks:     httpx.NewWebhookHandlers(whkSvc, log),
		OrgsSvc:      orgSvc,
		Log:          log,
		WebOrigin:    cfg.WebOrigin,
		DB:           pool,
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
