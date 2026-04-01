package service

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/model"
	"context"
)

// TaskCacheService — управление кэшем задач.
type TaskCacheService interface {
	SetTask(ctx context.Context, task domain.Task) error
	GetTask(ctx context.Context, id string) (domain.Task, error)
	DeleteCachedTask(ctx context.Context, id string) error
	GetList(ctx context.Context, key string) ([]model.TaskList, error)
	SetList(ctx context.Context, key string, tasks []model.TaskList) error
	InvalidateUserLists(ctx context.Context, userID string) error
}
