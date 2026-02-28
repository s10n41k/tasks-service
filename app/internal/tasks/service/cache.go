package service

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"context"
)

// TaskCacheService — управление кэшем задач.
type TaskCacheService interface {
	SetTask(ctx context.Context, task domain.Task) error
	GetTask(ctx context.Context, id string) (domain.Task, error)
	DeleteCachedTask(ctx context.Context, id string) error
	GetList(ctx context.Context, key string) ([]domain.Task, error)
	SetList(ctx context.Context, key string, tasks []domain.Task) error
	InvalidateUserLists(ctx context.Context, userID string) error
}
