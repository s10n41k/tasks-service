package main

import (
	"TODOLIST_Tasks/app/internal/config"
	tagsDB "TODOLIST_Tasks/app/internal/tags/db/postgres"
	tagsRedis "TODOLIST_Tasks/app/internal/tags/db/redis"
	tagsHandlers "TODOLIST_Tasks/app/internal/tags/handlers"
	tagsService "TODOLIST_Tasks/app/internal/tags/service"
	kafkaProducer "TODOLIST_Tasks/app/internal/tasks/event/kafka"
	"TODOLIST_Tasks/app/internal/tasks/handlers"
	outboxRepo "TODOLIST_Tasks/app/internal/tasks/repository/outbox"
	postgresRepo "TODOLIST_Tasks/app/internal/tasks/repository/postgres"
	redisRepo "TODOLIST_Tasks/app/internal/tasks/repository/redis"
	"TODOLIST_Tasks/app/internal/tasks/service"
	"TODOLIST_Tasks/app/internal/worker"
	"TODOLIST_Tasks/app/pkg/client/kafka"
	postgresql "TODOLIST_Tasks/app/pkg/client/postgres"
	redisclient "TODOLIST_Tasks/app/pkg/client/redis"
	"TODOLIST_Tasks/app/pkg/logging"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/julienschmidt/httprouter"
)

func main() {
	logger := logging.GetLogger()
	cfg, err := config.GetConfig()
	if err != nil {
		logger.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pgClient, err := postgresql.NewClient(ctx, 10, *cfg)
	if err != nil {
		logger.Fatal(err)
	}
	defer func() {
		pgClient.Close()
		logger.Info("Postgres connection closed")
	}()

	redisClient, err := redisclient.NewClient(ctx, 10, *cfg)
	if err != nil {
		logger.Fatal(err)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			logger.Errorf("close redis: %v", err)
		} else {
			logger.Info("Redis connection closed")
		}
	}()

	kafkaClient, err := kafka.NewClient(*cfg)
	if err != nil {
		logger.Fatal(err)
	}
	defer func() {
		if err := kafkaClient.Close(); err != nil {
			logger.Errorf("close kafka: %v", err)
		} else {
			logger.Info("Kafka connection closed")
		}
	}()

	// Инициализация репозиториев
	taskRepo := postgresRepo.NewRepository(pgClient, *logger)
	cacheRepo := redisRepo.NewRepository(redisClient)
	outbox := outboxRepo.NewRepository(pgClient)
	kafkaRepo := kafkaProducer.NewRepository(kafkaClient)

	// Tags (не рефакторим, оставляем как есть)
	tagRepo := tagsDB.NewRepository(pgClient, *logger)
	tagRedis := tagsRedis.NewRepositoryRedis(redisClient)

	// Инициализация сервисов
	cmd, query, cache := service.New(taskRepo, cacheRepo)
	tagSvc := tagsService.NewService(tagRepo, tagRedis)

	// Обработчики и роутер
	taskHandler := handlers.NewHandler(cmd, query, cache)
	tagHandler := tagsHandlers.NewHandler(tagSvc)

	router := httprouter.New()
	tagHandler.Register(router)
	taskHandler.Register(router)

	// Outbox worker
	processor := worker.NewProcessor(outbox, kafkaRepo, *logger)
	go processor.Start(ctx)

	logger.Info("starting application")
	start(router, cfg, logger)
}

func start(router *httprouter.Router, cfg *config.Config, logger *logging.Logger) {
	listenAddr := fmt.Sprintf("%s:%s", cfg.Listen.BindIP, cfg.Listen.Port)
	logger.Infof("starting server on %s", listenAddr)

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		logger.Fatal(err)
	}

	limiter := make(chan struct{}, 2500)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case limiter <- struct{}{}:
			defer func() { <-limiter }()
			router.ServeHTTP(w, r)
		default:
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"too many requests"}`))
		}
	})

	server := &http.Server{
		Handler:           handler,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		logger.Info("server started, ready to accept connections")
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal(err)
		}
	}()

	waitForShutdown(server, logger)
}

func waitForShutdown(server *http.Server, logger *logging.Logger) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	logger.Info("shutdown signal received, draining connections...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Errorf("server shutdown error: %v", err)
	} else {
		logger.Info("server shut down gracefully")
	}
}
