package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type PostgresPool struct {
	Pool *pgxpool.Pool
}

func NewPostgresPool(ctx context.Context, dsn string, maxConns, minConns int, maxLifetime time.Duration) (*PostgresPool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}

	config.MaxConns = int32(maxConns)
	config.MinConns = int32(minConns)
	config.MaxConnLifetime = maxLifetime

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &PostgresPool{Pool: pool}, nil
}

func (p *PostgresPool) Close() {
	p.Pool.Close()
}

type DBPool interface {
	Exec(ctx context.Context, sql string, args ...interface{}) (int64, error)
	Query(ctx context.Context, sql string, args ...interface{}) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) Row
	Begin(ctx context.Context) (Tx, error)
}

type Rows interface {
	Close()
	Next() bool
	Scan(dest ...interface{}) error
}

type Row interface {
	Scan(dest ...interface{}) error
}

type Tx interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
	Exec(ctx context.Context, sql string, args ...interface{}) (int64, error)
	Query(ctx context.Context, sql string, args ...interface{}) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) Row
}

func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgxpool.Tx) error, logger *zap.Logger) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				logger.Error("transaction rollback failed", zap.Error(rbErr))
			}
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			logger.Error("transaction rollback failed", zap.Error(rbErr))
		}
		return fmt.Errorf("transaction failed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
