package postgres

import (
	"TODOLIST_Tasks/app/internal/tasks/model"
	"context"
	"github.com/stretchr/testify/mock"
)

type TasksRepository struct {
	mock.Mock
}

func (m *TasksRepository) CreateTask(ctx context.Context, task model.Task) (string, error) {
	args := m.Called(ctx, task)
	return args.String(0), args.Error(1)
}

func (m *TasksRepository) UpdateTask(ctx context.Context, id string, task model.TaskUpdateDTO) (model.Task, error) {
	args := m.Called(ctx, id, task)
	return args.Get(0).(model.Task), args.Error(1)
}

func (m *TasksRepository) DeleteTask(ctx context.Context, id string) (string, error) {
	args := m.Called(ctx, id)
	return args.String(0), args.Error(1)
}

func (m *TasksRepository) FindOneTask(ctx context.Context, id string) (model.Task, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(model.Task), args.Error(1)
}

func (m *TasksRepository) FindAllTasks(ctx context.Context, sortOptions SortOptions,
	filterOptions FilterOptions, userId string) ([]model.Task, error) {
	args := m.Called(ctx, sortOptions, filterOptions, userId)
	return args.Get(0).([]model.Task), args.Error(1)
}

func (m *TasksRepository) FindAllByTag(ctx context.Context, userId string, tagId string) ([]model.Task, error) {
	args := m.Called(ctx, userId, tagId)
	return args.Get(0).([]model.Task), args.Error(1)

}
