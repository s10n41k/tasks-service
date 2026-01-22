package outbox

import (
	"TODOLIST_Tasks/app/internal/tasks/model"
	"context"
)

type Repository interface {
	GetUnprocessedEvents(ctx context.Context, limit int) ([]model.Event, error)
	MarkAsProcessed(ctx context.Context, id string) error
	MarkAsFailed(ctx context.Context, id string, errorMsg string) error
}
