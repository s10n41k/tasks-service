package kafka

import (
	"context"
	"github.com/segmentio/kafka-go"
	"time"
)

type Client struct {
	writer *kafka.Writer
}

func NewClient(ctx context.Context, brokers []string, maxAttempts int) (*Client, error) {
	var (
		client *Client
		err    error
	)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		client = &Client{
			writer: &kafka.Writer{
				Addr:         kafka.TCP(brokers...),
				Balancer:     &kafka.LeastBytes{},
				RequiredAcks: kafka.RequireAll, // гарантирует доставку
			},
		}

		// тестовое подключение
		testCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = client.writer.WriteMessages(testCtx,
			kafka.Message{
				Topic: "connection_test",
				Value: []byte("ping"),
			},
		)
		cancel()

		if err == nil {
			return client, nil
		}

		time.Sleep(time.Second * 2) // задержка между ретраями
	}

	return nil, err
}

func (c *Client) Produce(ctx context.Context, topic string, key []byte, value []byte) error {
	return c.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   key,
		Value: value,
	})
}

func (c *Client) Close() error {
	return c.writer.Close()
}
