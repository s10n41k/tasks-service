package port

import "context"

// OutboxRepository — контракт хранилища outbox-событий.
type OutboxRepository interface {
	GetUnprocessedEvents(ctx context.Context, limit int) ([]OutboxEvent, error)
	MarkAsProcessed(ctx context.Context, id string) error
	MarkBatchAsProcessed(ctx context.Context, ids []string) error
	MarkAsFailed(ctx context.Context, id string, errorMsg string) error
}

// OutboxEvent — доменное представление outbox-события.
type OutboxEvent struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	EventData     []byte
	Attempts      int
}
