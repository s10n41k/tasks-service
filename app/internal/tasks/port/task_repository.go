package port

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/api/sort"
	"context"
	"time"
)

// TaskRepository — контракт хранилища задач.
type TaskRepository interface {
	Create(ctx context.Context, task domain.Task) error
	CreateBatch(ctx context.Context, tasks []domain.Task) error
	Update(ctx context.Context, id string, patch UpdatePatch) (domain.Task, error)
	Delete(ctx context.Context, id string) error
	DeleteBatch(ctx context.Context, ids []string) error
	FindByID(ctx context.Context, id string) (domain.Task, error)
	FindByUser(ctx context.Context, userID string, sortOpts sort.Options, filterOpts filter.Option) ([]domain.Task, error)
	FindByTag(ctx context.Context, userID, tagID string) ([]domain.Task, error)
}

// UpdatePatch — поля для частичного обновления задачи.
type UpdatePatch struct {
	Title       *string
	Description *string
	Status      *domain.Status // уже разобранный тип, не строка
	Priority    *string
	DueDate     *time.Time
	TagName     *string
}
