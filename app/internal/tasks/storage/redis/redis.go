package redis

import (
	model2 "TODOLIST_Tasks/app/internal/tasks/model"
	"context"
)

type Repository interface {
	CacheTask(ctx context.Context, task model2.Task) error
	DeleteTaskCache(ctx context.Context, id string) error
	GetTaskFromCache(ctx context.Context, id string) (model2.Task, error)
	GetTasksFromCacheList(ctx context.Context, cacheKey string) ([]model2.Task, error)
	SetTasksToCacheList(ctx context.Context, cacheKey string, tasks []model2.Task) error
}
