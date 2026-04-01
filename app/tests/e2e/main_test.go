//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	taskBatch "TODOLIST_Tasks/app/internal/tasks/batch"
	sharedHandlers "TODOLIST_Tasks/app/internal/shared_tasks/handlers"
	sharedPostgres "TODOLIST_Tasks/app/internal/shared_tasks/repository/postgres"
	sharedService "TODOLIST_Tasks/app/internal/shared_tasks/service"
	tagsDB "TODOLIST_Tasks/app/internal/tags/db/postgres"
	tagsRedis "TODOLIST_Tasks/app/internal/tags/db/redis"
	tagsHandlers "TODOLIST_Tasks/app/internal/tags/handlers"
	tagsService "TODOLIST_Tasks/app/internal/tags/service"
	"TODOLIST_Tasks/app/internal/config"
	"TODOLIST_Tasks/app/internal/tasks/handlers"
	"TODOLIST_Tasks/app/internal/tasks/notification"
	postgresRepo "TODOLIST_Tasks/app/internal/tasks/repository/postgres"
	redisRepo "TODOLIST_Tasks/app/internal/tasks/repository/redis"
	"TODOLIST_Tasks/app/internal/tasks/service"
	postgresql "TODOLIST_Tasks/app/pkg/client/postgres"
	redisclient "TODOLIST_Tasks/app/pkg/client/redis"
	"TODOLIST_Tasks/app/pkg/logging"

	"github.com/julienschmidt/httprouter"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

const (
	testGatewaySign = "e2e-test-gateway-sign-secret"
	testUserID      = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	testUserID2     = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

var testServer *httptest.Server

// noopWsNotifier — заглушка WsNotifier: внешний users-service недоступен в тестах.
type noopWsNotifier struct{}

func (n *noopWsNotifier) Notify(_ context.Context, _, _ string, _ interface{}) error { return nil }

func TestMain(m *testing.M) {
	ctx := context.Background()
	logger := logging.GetLogger()

	// --- PostgreSQL container ---
	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpass"),
	)
	if err != nil {
		log.Fatalf("start postgres container: %v", err)
	}
	defer pgContainer.Terminate(ctx) //nolint:errcheck

	pgHost, err := pgContainer.Host(ctx)
	if err != nil {
		log.Fatalf("postgres host: %v", err)
	}
	pgPort, err := pgContainer.MappedPort(ctx, "5432/tcp")
	if err != nil {
		log.Fatalf("postgres port: %v", err)
	}

	// --- Redis container ---
	redisContainer, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		log.Fatalf("start redis container: %v", err)
	}
	defer redisContainer.Terminate(ctx) //nolint:errcheck

	redisHost, err := redisContainer.Host(ctx)
	if err != nil {
		log.Fatalf("redis host: %v", err)
	}
	redisPort, err := redisContainer.MappedPort(ctx, "6379/tcp")
	if err != nil {
		log.Fatalf("redis port: %v", err)
	}

	// --- Переменные окружения для конфига ---
	os.Setenv("GATEWAY_SIGN", testGatewaySign)
	os.Setenv("JWT_SECRET", "e2e-test-jwt-secret")
	os.Setenv("DB_TASKS_HOST", pgHost)
	os.Setenv("DB_TASKS_PORT", pgPort.Port())
	os.Setenv("DB_TASKS_DATABASE", "testdb")
	os.Setenv("DB_TASKS_USERNAME", "testuser")
	os.Setenv("DB_TASKS_PASSWORD", "testpass")
	os.Setenv("REDIS_TASKS_HOST", redisHost)
	os.Setenv("REDIS_TASKS_PORT", redisPort.Port())
	os.Setenv("TASKS_PORT", "0")
	os.Setenv("KAFKA_BROKER", "localhost:9092") // не используется — worker не запускается

	cfg, err := config.GetConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// --- Postgres client ---
	pgClient, err := postgresql.NewClient(ctx, 3, *cfg)
	if err != nil {
		log.Fatalf("pg client: %v", err)
	}
	defer pgClient.Close()

	// --- Миграции ---
	if err := runMigrations(ctx, pgClient); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	// --- Seed: подписки для тестовых пользователей (нужно для Propose shared task) ---
	for _, uid := range []string{testUserID, testUserID2} {
		pgClient.Exec(ctx, //nolint:errcheck
			`INSERT INTO user_subscriptions (user_id, has_subscription, expires_at)
			 VALUES ($1, true, NOW() + INTERVAL '1 year')
			 ON CONFLICT (user_id) DO NOTHING`,
			uid,
		)
	}

	// --- Redis client ---
	redisClient, err := redisclient.NewClient(ctx, 3, *cfg)
	if err != nil {
		log.Fatalf("redis client: %v", err)
	}
	defer redisClient.Close() //nolint:errcheck

	// --- Репозитории ---
	taskRepo := postgresRepo.NewRepository(pgClient, *logger)
	subtaskRepo := postgresRepo.NewSubtaskRepository(pgClient, *logger)
	subRepo := postgresRepo.NewSubscriptionRepository(pgClient, *logger)
	cacheRepo := redisRepo.NewRepository(redisClient)
	sharedRepo := sharedPostgres.New(pgClient, *logger)
	tagRepo := tagsDB.NewRepository(pgClient, *logger)
	tagRedisRepo := tagsRedis.NewRepositoryRedis(redisClient)

	// --- Сервисы ---
	cmd, query, cache, subtaskSvc, adminSvc := service.New(taskRepo, subtaskRepo, cacheRepo)
	tagSvc := tagsService.NewService(tagRepo, tagRedisRepo)
	sharedSvc := sharedService.New(sharedRepo)
	subSvc := service.NewSubscriptionService(subRepo)

	// Внешние клиенты — заглушки (не нужны для E2E тестов основных сценариев)
	wsNotifier := &noopWsNotifier{}
	usersMsg := notification.NewUsersMessageClient("localhost", "1")

	// --- Batch processor ---
	batchProc := taskBatch.New(cmd, cache, logger)
	batchProc.Start(ctx)

	// --- Обработчики ---
	taskHandler := handlers.NewHandler(cmd, query, cache, subtaskSvc, subSvc, adminSvc, batchProc, wsNotifier)
	tagHandler := tagsHandlers.NewHandler(tagSvc, cacheRepo)
	sharedHandler := sharedHandlers.New(sharedSvc, usersMsg, subSvc)

	// --- Роутер ---
	router := httprouter.New()
	tagHandler.Register(router)
	taskHandler.Register(router)
	sharedHandler.Register(router)

	// --- HTTP test server ---
	testServer = httptest.NewServer(router)
	defer testServer.Close()

	os.Exit(m.Run())
}

// runMigrations применяет все *.up.sql миграции в порядке имён файлов.
// 0008 пропускается — он заменён миграцией 0009.
func runMigrations(ctx context.Context, db postgresql.Client) error {
	// app/tests/e2e → ../../migrations
	migrationsDir := filepath.Join("..", "..", "migrations")
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
	if err != nil {
		return fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(files)

	for _, f := range files {
		if strings.Contains(filepath.Base(f), "0008_") {
			continue
		}
		content, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		if _, err := db.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("exec %s: %w", f, err)
		}
	}
	return nil
}
