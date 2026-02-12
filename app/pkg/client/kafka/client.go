package kafka

import (
	"TODOLIST_Tasks/app/internal/config"
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	topic            = "events_task"
	maxRetries       = 3
	retryInterval    = 500 * time.Millisecond
	writerBatchSize  = 1000
	writerBatchBytes = 1048576 // 1MB
	writerBatchTime  = 100     // ms
	statsInterval    = 30 * time.Second
)

type Client interface {
	Write(ctx context.Context, key string, msg []byte) error
	WriteSync(ctx context.Context, key string, msg []byte) error
	GetStats() (messages, errors uint64)
	Close() error
}

type client struct {
	writer        *kafka.Writer
	brokers       []string
	statsTicker   *time.Ticker
	statsDone     chan struct{}
	messageCount  uint64
	errorCount    uint64
	lastStatsTime time.Time
}

func NewClient(cfg config.Config) (Client, error) {
	c := &client{
		brokers:       cfg.Kafka.Brokers,
		statsTicker:   time.NewTicker(statsInterval),
		statsDone:     make(chan struct{}),
		lastStatsTime: time.Now(),
		writer: &kafka.Writer{
			Addr:     kafka.TCP(cfg.Kafka.Brokers...),
			Topic:    topic,
			Balancer: &kafka.Hash{},

			// ⚡ КРИТИЧЕСКИЕ ОПТИМИЗАЦИИ ДЛЯ СКОРОСТИ
			BatchSize:    writerBatchSize,                    // 1000 сообщений в батче
			BatchBytes:   writerBatchBytes,                   // 1MB максимальный размер батча
			BatchTimeout: writerBatchTime * time.Millisecond, // Ждать до 100мс

			RequiredAcks: kafka.RequireOne, // Только leader должен подтвердить
			Async:        true,             // ⚡ АСИНХРОННАЯ ЗАПИСЬ!
			Compression:  kafka.Snappy,     // Сжатие для экономии трафика

			MaxAttempts:     maxRetries, // 3 попытки
			WriteBackoffMin: 100 * time.Millisecond,
			WriteBackoffMax: 1 * time.Second,
			WriteTimeout:    30 * time.Second,
			ReadTimeout:     30 * time.Second,

			// Отладка (можно отключить в продакшене)
			Logger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
				log.Printf("Kafka Debug: %s", fmt.Sprintf(msg, args...))
			}),
		},
	}

	// Проверка соединения
	if err := c.ping(); err != nil {
		return nil, fmt.Errorf("kafka not available: %w", err)
	}

	// Запускаем обработчик статистики
	go c.statsHandler()

	log.Printf("Kafka async client initialized. Brokers: %v, BatchSize: %d, Async: %v",
		c.brokers, writerBatchSize, true)
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

// Write отправляет сообщение асинхронно
func (c *client) Write(ctx context.Context, key string, msg []byte) error {
	atomic.AddUint64(&c.messageCount, 1)

	err := c.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: msg,
		Time:  time.Now().UTC(),
	})

	// Только немедленные ошибки (например, контекст отменён)
	if err != nil {
		atomic.AddUint64(&c.errorCount, 1)
		return fmt.Errorf("immediate write error: %w", err)
	}

	return nil
}

// WriteSync синхронная версия для критичных операций
func (c *client) WriteSync(ctx context.Context, key string, msg []byte) error {
	// Создаём временный синхронный writer
	syncWriter := &kafka.Writer{
		Addr:         kafka.TCP(c.brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll, // Гарантированная доставка
		Async:        false,            // Синхронный режим
		MaxAttempts:  5,
	}
	defer syncWriter.Close()

	return syncWriter.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: msg,
		Time:  time.Now().UTC(),
	})
}

// statsHandler периодически логирует статистику
func (c *client) statsHandler() {
	defer c.statsTicker.Stop()

	for {
		select {
		case <-c.statsTicker.C:
			messages := atomic.LoadUint64(&c.messageCount)
			errors := atomic.LoadUint64(&c.errorCount)
			elapsed := time.Since(c.lastStatsTime).Seconds()

			rate := float64(0)
			if elapsed > 0 {
				rate = float64(messages) / elapsed
			}

			log.Printf("📊 Kafka Stats [last %.0fs]: Messages=%d (%.1f/сек), Errors=%d, ErrorRate=%.2f%%",
				elapsed, messages, rate, errors,
				float64(errors)/float64(messages+1)*100)

			// Сбрасываем счётчики для следующего интервала
			atomic.StoreUint64(&c.messageCount, 0)
			atomic.StoreUint64(&c.errorCount, 0)
			c.lastStatsTime = time.Now()

		case <-c.statsDone:
			return
		}
	}
}

// GetStats возвращает текущую статистику
func (c *client) GetStats() (messages, errors uint64) {
	return atomic.LoadUint64(&c.messageCount), atomic.LoadUint64(&c.errorCount)
}

// Close корректно закрывает клиент
func (c *client) Close() error {
	close(c.statsDone)

	// Закрываем с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Используем контекст для закрытия
	errChan := make(chan error, 1)
	go func() {
		errChan <- c.writer.Close()
	}()

	select {
	case err := <-errChan:
		if err != nil {
			log.Printf("Error closing Kafka writer: %v", err)
			return err
		}
		log.Printf("Kafka client closed gracefully")
		return nil
	case <-ctx.Done():
		log.Printf("Kafka client close timeout after 10s, forcing close")
		// Принудительное закрытие, если не успел
		return ctx.Err()
	}
}
