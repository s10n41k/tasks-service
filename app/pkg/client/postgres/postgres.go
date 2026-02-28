// client/postgres/pgxClient.go
package postgres

import (
	"TODOLIST_Tasks/app/internal/config"
	"TODOLIST_Tasks/app/pkg/utils/repeatable"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type Client interface {
	Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row

	// Transaction support
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
	Begin(ctx context.Context) (pgx.Tx, error)

	// COPY support - ДОБАВЛЕНО!
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)

	Close()
	Pool() *pgxpool.Pool
	PrintStats()
}

type pgxClient struct {
	pool *pgxpool.Pool
}

func (c *pgxClient) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	return c.pool.Exec(ctx, query, args...)
}

func (c *pgxClient) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	return c.pool.Query(ctx, query, args...)
}

func (c *pgxClient) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return c.pool.QueryRow(ctx, query, args...)
}

func (c *pgxClient) Begin(ctx context.Context) (pgx.Tx, error) {
	return c.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted,
	})
}

func (c *pgxClient) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	return c.pool.BeginTx(ctx, opts)
}

// ДОБАВЛЕНО: CopyFrom через пул
func (c *pgxClient) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return c.pool.CopyFrom(ctx, tableName, columnNames, rowSrc)
}

func (c *pgxClient) Close() {
	c.pool.Close()
}

func (c *pgxClient) Pool() *pgxpool.Pool {
	return c.pool
}

func (c *pgxClient) PrintStats() {
	stats := c.pool.Stat()
	log.Printf(
		"PG POOL STATS: Total=%d Idle=%d Acquired=%d AcquireCount=%d Wait=%s",
		stats.TotalConns(),
		stats.IdleConns(),
		stats.AcquiredConns(),
		stats.AcquireCount(),
		stats.AcquireDuration(),
	)
}

// NewClient остается без изменений
func NewClient(ctx context.Context, maxAttempts int, sc config.Config) (Client, error) {
	usePgBouncer := sc.Postgres.Port == "6432" || sc.Postgres.Host == "pgbouncer"

	dsn := fmt.Sprintf(
		"postgresql://%s:%s@%s:%s/%s?sslmode=disable",
		sc.Postgres.Username,
		sc.Postgres.Password,
		sc.Postgres.Host,
		sc.Postgres.Port,
		sc.Postgres.Database,
	)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pg config: %w", err)
	}

	// При 5000 RPS с транзакцией ~2ms нужно ~10 конкурентных соединений
	// 200 — запас на пики и медленные запросы
	poolConfig.MaxConns = 200
	poolConfig.MinConns = 20
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnLifetimeJitter = 5 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.HealthCheckPeriod = 15 * time.Second

	if !usePgBouncer {
		poolConfig.ConnConfig.RuntimeParams["jit"] = "off"
		poolConfig.ConnConfig.RuntimeParams["work_mem"] = "16MB"
		poolConfig.ConnConfig.RuntimeParams["effective_cache_size"] = "4GB"
		// statement_timeout: 5с — быстрый фейл под нагрузкой вместо зависания
		poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = "5000"
		// lock_timeout: не ждать блокировки дольше 2с
		poolConfig.ConnConfig.RuntimeParams["lock_timeout"] = "2000"
		poolConfig.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "10000"
		// synchronous_commit=off даёт 5-10x прирост throughput на INSERT
		poolConfig.ConnConfig.RuntimeParams["synchronous_commit"] = "off"
	} else {
		poolConfig.ConnConfig.RuntimeParams = make(map[string]string)
		poolConfig.ConnConfig.RuntimeParams["application_name"] = "tasks-service"
		poolConfig.ConnConfig.PreferSimpleProtocol = true
	}

	poolConfig.ConnConfig.ConnectTimeout = 2 * time.Second

	var pool *pgxpool.Pool

	err = repeatable.DoWithTries(func() error {
		ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		pool, err = pgxpool.ConnectConfig(ctxTimeout, poolConfig)
		if err != nil {
			return fmt.Errorf("connect to pg: %w", err)
		}

		return pool.Ping(ctxTimeout)
	}, maxAttempts, 5*time.Second)

	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres after %d attempts: %w", maxAttempts, err)
	}

	return &pgxClient{pool: pool}, nil
}
