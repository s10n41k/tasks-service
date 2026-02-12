package config

import (
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
	JWTSecret     string
	GatewaySecret string
	GatewaySign   string
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

	// Secrets
	cfg.JWTSecret = getEnv("JWT_SECRET", "uYk3Pq7RvA1wXzJ5LmN9tBcDfGhJkMnP2S4V6Y8ZaCd")
	cfg.GatewaySecret = getEnv("GATEWAY_SIGN", "68b329da9893e34099c7d8ad5cb9c940")

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
