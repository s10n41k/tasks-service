package kafka

import (
	"TODOLIST_Tasks/app/internal/config"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	topic           = "events_task"
	maxRetries      = 3
	retryInterval   = 500 * time.Millisecond
	writerBatchSize = 100
)

type client struct {
	writer  *kafka.Writer
	brokers []string
}

func NewClient(cfg config.KafkaConfig) (Client, error) {
	c := &client{
		brokers: cfg.Brokers,
		writer: &kafka.Writer{
			Addr:         kafka.TCP(cfg.Brokers...),
			Topic:        topic,
			Balancer:     &kafka.Hash{},
			BatchSize:    writerBatchSize,
			RequiredAcks: kafka.RequireOne,
			Async:        false, // Синхронная запись для надежности
		},
	}

	if err := c.ping(); err != nil {
		return nil, fmt.Errorf("kafka not available: %w", err)
	}

	log.Println("Kafka client successfully connected to brokers:", c.brokers)
	return c, nil
}

func (c *client) ping() error {
	conn, err := kafka.Dial("tcp", c.brokers[0])
	if err != nil {
		return fmt.Errorf("cannot connect to broker %s: %w", c.brokers[0], err)
	}
	defer conn.Close()

	_, err = conn.ReadPartitions()
	return err
}

func (c *client) Write(ctx context.Context, key string, msg []byte) error {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := c.writer.WriteMessages(ctx, kafka.Message{
			Key:   []byte(key),
			Value: msg,
			Time:  time.Now().UTC(),
		})

		if err == nil {
			return nil
		}

		lastErr = err
		log.Printf("Kafka write attempt %d/%d failed: %v", attempt, maxRetries, err)

		if attempt < maxRetries {
			time.Sleep(retryInterval)
		}
	}

	return fmt.Errorf("failed to write message after %d attempts: %w", maxRetries, lastErr)
}

func (c *client) Close() error {
	return c.writer.Close()
}
