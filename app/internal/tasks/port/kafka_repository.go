package port

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"context"
)

// KafkaRepository — контракт producer Kafka.
type KafkaRepository interface {
	Write(ctx context.Context, task domain.Task, eventType string) error
	Close() error
}
