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

// SubtaskService — операции над подзадачами обычных задач.
type SubtaskService interface {
	AddSubtask(ctx context.Context, taskID, ownerID, title string) (domain.Subtask, error)
	ToggleSubtaskDone(ctx context.Context, subtaskID, taskOwnerID string) (domain.Subtask, error)
	DeleteSubtask(ctx context.Context, subtaskID, taskOwnerID string) error
	UpdateSubtask(ctx context.Context, subtaskID, taskOwnerID, title string) error
}
