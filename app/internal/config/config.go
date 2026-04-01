package config

import (
	"log"
	"os"
	"strings"
)

type Config struct {
	Listen struct {
		Type   string
		Port   string
		BindIP string
	}
	Postgres struct {
		Host     string
		Port     string
		Database string
		Username string
		Password string
	}
	Redis struct {
		Host     string
		Port     string
		Password string
		Protocol string
	}
	Kafka struct {
		Brokers []string
		GroupID string
	}
	Gateway struct {
		Host string
		Port string
	}
	UsersService struct {
		Host string
		Port string
	}
	JWTSecret   string
	GatewaySign string
}

func GetConfig() (*Config, error) {
	cfg := &Config{}

	// Listen
	cfg.Listen.Port = getEnv("TASKS_PORT", "8000")
	cfg.Listen.BindIP = getEnv("LISTEN_BIND_IP", "0.0.0.0")
	cfg.Listen.Type = "port"

	// PostgreSQL
	cfg.Postgres.Host = getEnv("DB_TASKS_HOST", "postgres-tasks")
	cfg.Postgres.Port = getEnv("DB_TASKS_PORT", "5433")
	cfg.Postgres.Database = getEnv("DB_TASKS_DATABASE", "mydatabase1")
	cfg.Postgres.Username = getEnv("DB_TASKS_USERNAME", "user1")
	cfg.Postgres.Password = getEnv("DB_TASKS_PASSWORD", "password1")

	// Redis
	cfg.Redis.Host = getEnv("REDIS_TASKS_HOST", "redis-tasks")
	cfg.Redis.Port = getEnv("REDIS_TASKS_PORT", "6379")
	cfg.Redis.Password = getEnv("REDIS_TASKS_PASSWORD", "")
	cfg.Redis.Protocol = "tcp"

	// Kafka
	brokers := getEnv("KAFKA_BROKER", "kafka:9092")
	cfg.Kafka.Brokers = strings.Split(brokers, ",")
	cfg.Kafka.GroupID = getEnv("KAFKA_GROUP_TASKS", "tasks-service-group")

	// Gateway (для notification client)
	cfg.Gateway.Host = getEnv("GATEWAY_HOST", "gateway")
	cfg.Gateway.Port = getEnv("GATEWAY_PORT", "8080")

	// Users service (для авто-сообщений при изменении совместных задач)
	cfg.UsersService.Host = getEnv("USERS_SERVICE_HOST", "users-service")
	cfg.UsersService.Port = getEnv("USERS_SERVICE_PORT", "8001")

	// Секреты — обязательные переменные окружения, дефолты недопустимы
	cfg.JWTSecret = requireEnv("JWT_SECRET")
	cfg.GatewaySign = requireEnv("GATEWAY_SIGN")

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// requireEnv возвращает значение env-переменной или завершает процесс с ошибкой.
func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("обязательная переменная окружения %s не задана", key)
	}
	return v
}
