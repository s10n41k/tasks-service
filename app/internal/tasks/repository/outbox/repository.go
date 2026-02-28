package outbox

import (
	"TODOLIST_Tasks/app/internal/tasks/port"
	postgresql "TODOLIST_Tasks/app/pkg/client/postgres"
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
)

type outboxRepo struct {
	client  postgresql.Client
	builder sq.StatementBuilderType
}

func NewRepository(client postgresql.Client) port.OutboxRepository {
	return &outboxRepo{
		client:  client,
		builder: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

func (r *outboxRepo) GetUnprocessedEvents(ctx context.Context, limit int) ([]port.OutboxEvent, error) {
	query, args, err := r.builder.
		Select("id", "aggregate_type", "aggregate_id", "event_type", "event_data", "attempts").
		From("outbox_events").
		Where(sq.Eq{"processed_at": nil}).
		OrderBy("created_at ASC").
		Limit(uint64(limit)).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select outbox: %w", err)
	}

	rows, err := r.client.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query outbox: %w", err)
	}
	defer rows.Close()

	var events []port.OutboxEvent
	for rows.Next() {
		var e port.OutboxEvent
		var data []byte
		if err := rows.Scan(&e.ID, &e.AggregateType, &e.AggregateID, &e.EventType, &data, &e.Attempts); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		e.EventData = data
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox rows error: %w", err)
	}
	return events, nil
}

func (r *outboxRepo) MarkAsProcessed(ctx context.Context, id string) error {
	query, args, err := r.builder.
		Update("outbox_events").
		Set("processed_at", time.Now().UTC()).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build mark processed: %w", err)
	}
	res, err := r.client.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("outbox event %s не найдено", id)
	}
	return nil
}

func (r *outboxRepo) MarkBatchAsProcessed(ctx context.Context, ids []string) error {
	_, err := r.client.Exec(ctx,
		`UPDATE outbox_events SET processed_at = $1 WHERE id = ANY($2::uuid[])`,
		time.Now().UTC(), ids,
	)
	return err
}

func (r *outboxRepo) MarkAsFailed(ctx context.Context, id string, errorMsg string) error {
	query, args, err := r.builder.
		Update("outbox_events").
		Set("attempts", sq.Expr("attempts + 1")).
		Set("last_error", errorMsg).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build mark failed: %w", err)
	}
	res, err := r.client.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("outbox event %s не найдено", id)
	}
	return nil
}
