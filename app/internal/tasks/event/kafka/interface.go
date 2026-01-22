package kafka

import (
	"TODOLIST_Tasks/app/internal/tasks/model"
	"context"
)

type Repository interface {
	Write(ctx context.Context, task model.Task, eventType string) error
	Close() error
}
