package main

import (
	"TODOLIST_Tasks/app/internal/config"
	db2 "TODOLIST_Tasks/app/internal/tags/db/postgres"
	redis3 "TODOLIST_Tasks/app/internal/tags/db/redis"
	handlers2 "TODOLIST_Tasks/app/internal/tags/handlers"
	service2 "TODOLIST_Tasks/app/internal/tags/service"
	"TODOLIST_Tasks/app/internal/tasks/db/outbox"
	"TODOLIST_Tasks/app/internal/tasks/db/postgres"
	redis2 "TODOLIST_Tasks/app/internal/tasks/db/redis"
	kafka2 "TODOLIST_Tasks/app/internal/tasks/event/kafka"
	"TODOLIST_Tasks/app/internal/tasks/handlers"
	"TODOLIST_Tasks/app/internal/tasks/service"
	"TODOLIST_Tasks/app/internal/worker"
	"TODOLIST_Tasks/app/pkg/client/kafka"
	postgresql "TODOLIST_Tasks/app/pkg/client/postgres"
	"TODOLIST_Tasks/app/pkg/client/redis"
	"TODOLIST_Tasks/app/pkg/logging"
	"context"
	"errors"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	logger := logging.GetLogger()
	cfg, err := config.GetConfig()
	if err != nil {
		logger.Fatal(err)
	}

	// Инициализация клиентов БД с контекстом
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientPostgresTasks, err := postgresql.NewClient(ctx, 10, *cfg)
	if err != nil {
		logger.Fatal(err)
	}
	defer func() {
		clientPostgresTasks.Close()
		logger.Info("Postgres is closed")
	}()

	clientRedis, err := redis.NewClient(ctx, 10, *cfg)
	if err != nil {
		logger.Fatal(err)
	}
	defer func() {
		if err := clientRedis.Close(); err != nil {
			logger.Errorf("failed to close redis connection: %v", err)
			logger.Info("Redis is closed")
		}
	}()

	clientKafka, err := kafka.NewClient(*cfg)
	if err != nil {
		logger.Fatal(err)
	}
	defer func() {
		if err := clientKafka.Close(); err != nil {
			logger.Errorf("failed to close kafka connection: %v", err)
			logger.Info("Kafka is closed")
		}
	}()

	// Инициализация репозиториев и сервисов
	repositoryRedisTasks := redis2.NewRepositoryRedis(clientRedis)
	repositoryRedisTags := redis3.NewRepositoryRedis(clientRedis)
	repositoryTags := db2.NewRepository(clientPostgresTasks)
	repositoryTasks := postgres.NewRepository(clientPostgresTasks, *logger)
	producerKafka := kafka2.NewRepository(clientKafka)
	repositoryOutbox := outbox.NewOutboxRepository(clientPostgresTasks)
	processor := worker.NewProcessor(repositoryOutbox, producerKafka, *logger)
	newServiceTags := service2.NewService(repositoryTags, repositoryRedisTags)
	newServiceTasks := service.NewService(repositoryTasks, repositoryTags, repositoryRedisTasks)

	// Инициализация обработчиков
	handlerTasks := handlers.NewHandler(newServiceTasks)
	handlerTags := handlers2.NewHandler(newServiceTags)
	router := httprouter.New()
	handlerTags.Register(router)
	handlerTasks.Register(router)

	logger.Info("starting application")

	go processor.Start(ctx)

	// Запуск сервера с graceful shutdown
	start(router, cfg, logger)
}

func start(
	router *httprouter.Router,
	cfg *config.Config,
	logger *logging.Logger,
) {
	listenAddr := fmt.Sprintf("%s:%s", cfg.Listen.BindIP, cfg.Listen.Port)
	logger.Infof("🚀 STARTING HIGH LOAD SERVER ON %s", listenAddr)

	// TCP listener
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		logger.Fatal(err)
	}

	// HTTP сервер с минимальными таймаутами
	server := &http.Server{
		Handler:           router,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    0, // без лимита
	}

	// Лимитер одновременных горутин (10k)
	limiter := make(chan struct{}, 500)
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case limiter <- struct{}{}:
			defer func() { <-limiter }()
			router.ServeHTTP(w, r)
		default:
			// быстрый отказ при перегрузке
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("429 Too Many Requests"))
		}
	})

	logger.Info("🔥 SERVER STARTED - READY FOR HIGH LOAD")
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal(err)
	}
	waitForShutdown(server, logger)
}

// 🔥 7. ДОБАВЬ ГЛОБАЛЬНЫЙ СЧЁТЧИК
var requestCounter int64

func countMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCounter, 1)
		next.ServeHTTP(w, r)
	})
}

// 🔥 8. УТИЛИТА ДЛЯ ПАМЯТИ
func getMemoryMB() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc / 1024 / 1024
}

// 🔥 9. УПРОЩЁННЫЙ SHUTDOWN
func waitForShutdown(server *http.Server, logger *logging.Logger) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	logger.Info("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server.Shutdown(ctx)
}
