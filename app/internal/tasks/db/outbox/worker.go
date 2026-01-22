package outbox

import (
	model2 "TODOLIST_Tasks/app/internal/tasks/model"
	"TODOLIST_Tasks/app/internal/tasks/storage/outbox"
	"TODOLIST_Tasks/app/pkg/client/postgres"
	"context"
	"fmt"
	sq "github.com/Masterminds/squirrel"
	"time"
)

type outboxRepository struct {
	client  postgres.Client
	builder sq.StatementBuilderType
}

func NewOutboxRepository(client postgres.Client) outbox.Repository {
	return &outboxRepository{
		client:  client,
		builder: sq.StatementBuilder.PlaceholderFormat(sq.Dollar), // ✅ ДОБАВЛЕНО
	}
}

// GetUnprocessedEvents возвращает необработанные события
func (r *outboxRepository) GetUnprocessedEvents(ctx context.Context, limit int) ([]model2.Event, error) {
	query, args, err := r.builder. // ✅ ИСПОЛЬЗУЕМ builder вместо sq
					Select("id", "aggregate_type", "aggregate_id", "event_type", "event_data", "created_at", "attempts", "last_error").
					From("outbox_events").
					Where(sq.Eq{"processed_at": nil}).
					OrderBy("created_at ASC").
					Limit(uint64(limit)).
					ToSql()

	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := r.client.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query unprocessed events: %w", err)
	}
	defer rows.Close()

	var events []model2.Event
	for rows.Next() {
		var event model2.Event
		var eventData []byte

		err := rows.Scan(
			&event.ID,
			&event.AggregateType,
			&event.AggregateID,
			&event.EventType,
			&eventData,
			&event.CreatedAt,
			&event.Attempts,
			&event.LastError,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		event.EventData = eventData
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return events, nil
}

// MarkAsProcessed помечает событие как обработанное
func (r *outboxRepository) MarkAsProcessed(ctx context.Context, id string) error {
	query, args, err := r.builder. // ✅ ИСПОЛЬЗУЕМ builder вместо sq
					Update("outbox_events").
					Set("processed_at", time.Now().UTC()).
					Where(sq.Eq{"id": id}).
					ToSql()

	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	// ДЕБАГ: посмотрим какой SQL генерируется
	fmt.Printf("DEBUG MarkAsProcessed SQL: %s\n", query)
	fmt.Printf("DEBUG MarkAsProcessed Args: %v\n", args)

	result, err := r.client.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to mark event as processed: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("event with id %s not found", id)
	}

	return nil
}

// MarkAsFailed помечает событие как неудачное
func (r *outboxRepository) MarkAsFailed(ctx context.Context, id string, errorMsg string) error {
	query, args, err := r.builder. // ✅ ИСПОЛЬЗУЕМ builder вместо sq
					Update("outbox_events").
					Set("attempts", sq.Expr("attempts + 1")).
					Set("last_error", errorMsg).
					Where(sq.Eq{"id": id}).
					ToSql()

	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := r.client.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to mark event as failed: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("event with id %s not found", id)
	}

	return nil
}
