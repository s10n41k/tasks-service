package port

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/model"
	"context"
)

// CacheRepository — контракт кэша задач.
type CacheRepository interface {
	SetTask(ctx context.Context, task domain.Task) error
	GetTask(ctx context.Context, id string) (domain.Task, error)
	DeleteTask(ctx context.Context, id string) error
	GetList(ctx context.Context, key string) ([]model.TaskList, error)
	SetList(ctx context.Context, key string, tasks []model.TaskList) error
	InvalidateUserLists(ctx context.Context, userID string) error
	// InvalidateTagTasks удаляет кэши всех задач, привязанных к указанному тегу.
	InvalidateTagTasks(ctx context.Context, tagID string) error
	// InvalidateUserTaskCaches удаляет кэши ВСЕХ задач пользователя.
	// Используется при изменении/удалении тега: так как TagID может быть nil в момент
	// кэширования задачи (CTE резолвит тег в БД, не возвращая ID), tracking-сет
	// tag_tasks:{tagID} ненадёжен. Надёжнее сбросить все задачи пользователя.
	InvalidateUserTaskCaches(ctx context.Context, userID string) error
}
