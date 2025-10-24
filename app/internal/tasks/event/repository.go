package event

import (
	"TODOLIST_Tasks/app/internal/tasks/model"
	"context"
)

type Repository interface {
	TaskCreated(ctx context.Context, task model.Task) error
	TaskUpdated(ctx context.Context, task model.Task) error
	TaskDeleted(ctx context.Context, id string) error
}
