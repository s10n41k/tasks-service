package service

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/port"
	"context"
)

// TaskCommandService — операции записи (изменение состояния задач).
type TaskCommandService interface {
	CreateTask(ctx context.Context, task domain.Task) error
	CreateTaskBatch(ctx context.Context, tasks []domain.Task) error
	UpdateTask(ctx context.Context, id string, patch port.UpdatePatch) (domain.Task, error)
	DeleteTask(ctx context.Context, id string) error
	DeleteTaskBatch(ctx context.Context, ids []string) error
}
