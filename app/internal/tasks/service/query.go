package service

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/api/sort"
	"context"
)

// TaskQueryService — операции чтения задач.
type TaskQueryService interface {
	FindTask(ctx context.Context, id string) (domain.Task, error)
	FindTasksByUser(ctx context.Context, userID string, sortOpts sort.Options, filterOpts filter.Option) ([]domain.Task, error)
	FindTasksByTag(ctx context.Context, userID, tagID string) ([]domain.Task, error)
}
