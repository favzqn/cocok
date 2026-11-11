package platform

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // register "pgx" database/sql driver for goose
	"github.com/pressly/goose/v3"
)

func OpenPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate applies embedded goose migrations from dir (e.g. "migrations").
func Migrate(ctx context.Context, pool *pgxpool.Pool, dir fs.FS) error {
	db, err := goose.OpenDBWithDriver("pgx", pool.Config().ConnString())
	if err != nil {
		return err
	}
	defer db.Close()
	goose.SetBaseFS(dir)
	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

func ClosePool(pool *pgxpool.Pool, log *slog.Logger) {
	if pool != nil {
		pool.Close()
		log.Info("database pool closed")
	}
}

var ErrNotFound = errors.New("not found")

// ErrValidation marks user-input validation failures (map to 400).
var ErrValidation = errors.New("validation failed")
