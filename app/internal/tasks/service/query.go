package service

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/model"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/api/sort"
	"context"
)

// TaskQueryService — операции чтения задач.
type TaskQueryService interface {
	// FindTask возвращает задачу и флаг fromCache: true — из Redis, false — из БД.
	FindTask(ctx context.Context, id string) (domain.Task, bool, error)
	FindTasksByUser(ctx context.Context, userID string, sortOpts sort.Options, filterOpts filter.Option) ([]model.TaskList, error)
	FindTasksByTag(ctx context.Context, userID, tagID string) ([]model.TaskList, error)
	AdminFindAllTasks(ctx context.Context) ([]model.TaskList, error)
	AdminFindAllFiltered(ctx context.Context, from, to, status, priory string) ([]model.TaskList, error)
}

// TaskAdminService — admin-операции над задачами.
type TaskAdminService interface {
	AdminSoftDelete(ctx context.Context, id string) (string, error)
	AcknowledgeAdminDeletion(ctx context.Context, id, userID string) error
	AdminSoftDeleteShared(ctx context.Context, id string) (string, string, string, error)
	AcknowledgeAdminDeletionShared(ctx context.Context, id, userID string) error
	AdminRestore(ctx context.Context, id string) (string, error)
	AdminRestoreShared(ctx context.Context, id string) (string, string, error)
}
