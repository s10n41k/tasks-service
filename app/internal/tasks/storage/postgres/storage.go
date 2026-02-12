package postgres

import (
	model2 "TODOLIST_Tasks/app/internal/tasks/model"
	"context"
)

type Repository interface {
	CreateTask(ctx context.Context, task model2.Task) error
	UpdateTask(ctx context.Context, id string, task model2.TaskUpdateDTO) (model2.Task, error)
	DeleteTask(ctx context.Context, id string) (string, error)
	FindOneTask(ctx context.Context, id string) (model2.Task, error)
	FindAllTasks(ctx context.Context, sortOptions SortOptions, filterOptions FilterOptions, userId string) ([]model2.Task, error)
	FindAllByTag(ctx context.Context, userId string, tagId string) ([]model2.Task, error)
}

type SortOptions interface {
	GetOrderBy() string
}

type FilterOptions interface {
	CreateQuery() string
}
