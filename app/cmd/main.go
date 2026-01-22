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
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	logger := logging.GetLogger()
	cfg := config.GetConfig()

	// Инициализация клиентов БД с контекстом
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientPostgresTasks, err := postgresql.NewClient(ctx, 10, cfg.StoragePostgresConfig)
	if err != nil {
		logger.Fatal(err)
	}
	defer func() {
		clientPostgresTasks.Close()
		logger.Info("Postgres is closed")
	}()

	clientRedis, err := redis.NewClient(ctx, 10, cfg.StorageRedisConfig)
	if err != nil {
		logger.Fatal(err)
	}
	defer func() {
		if err := clientRedis.Close(); err != nil {
			logger.Errorf("failed to close redis connection: %v", err)
			logger.Info("Redis is closed")
		}
	}()

	clientKafka, err := kafka.NewClient(cfg.KafkaConfig)
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
	repositoryTasks := postgres.NewRepository(clientPostgresTasks)
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
	var listener net.Listener
	var listenErr error

	if cfg.ListenConfig.Type == "sock" {
		logger.Info("detect app path")
		appDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
		if err != nil {
			logger.Fatal(err)
		}
		socketPath := path.Join(appDir, "app.sock")
		logger.Infof("listen unix socket: %s", socketPath)
		listener, listenErr = net.Listen("unix", socketPath)
	} else {
		listenAddr := fmt.Sprintf("%s:%s", cfg.ListenConfig.BindIP, cfg.ListenConfig.Port)
		logger.Infof("listen tcp on %s", listenAddr)
		listener, listenErr = net.Listen("tcp", listenAddr)
	}

	if listenErr != nil {
		logger.Fatal(listenErr)
	}

	server := &http.Server{
		Handler:      router,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	// Канал для получения ошибок от сервера
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server is starting...")
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Канал для сигналов ОС
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Ожидаем либо сигнал завершения, либо ошибку сервера
	select {
	case err := <-serverErr:
		logger.Fatalf("server error: %v", err)
	case sig := <-sigChan:
		logger.Infof("received signal: %v, initiating shutdown...", sig)

		// Таймаут для graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Остановка сервера
		if err := server.Shutdown(ctx); err != nil {
			logger.Errorf("server shutdown error: %v", err)
		}

		logger.Info("server stopped gracefully")
	}
}
