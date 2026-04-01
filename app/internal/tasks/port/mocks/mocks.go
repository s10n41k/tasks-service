package mocks

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/model"
	"TODOLIST_Tasks/app/internal/tasks/port"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/api/sort"
	"context"

	"github.com/stretchr/testify/mock"
)

// TaskRepository — мок репозитория задач.
type TaskRepository struct{ mock.Mock }

func (m *TaskRepository) Create(ctx context.Context, task domain.Task) error {
	return m.Called(ctx, task).Error(0)
}
func (m *TaskRepository) CreateBatch(ctx context.Context, tasks []domain.Task) error {
	return m.Called(ctx, tasks).Error(0)
}
func (m *TaskRepository) Update(ctx context.Context, id string, patch port.UpdatePatch) (domain.Task, error) {
	args := m.Called(ctx, id, patch)
	return args.Get(0).(domain.Task), args.Error(1)
}
func (m *TaskRepository) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
func (m *TaskRepository) DeleteBatch(ctx context.Context, ids []string) error {
	return m.Called(ctx, ids).Error(0)
}
func (m *TaskRepository) FindByID(ctx context.Context, id string) (domain.Task, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(domain.Task), args.Error(1)
}
func (m *TaskRepository) FindByUser(ctx context.Context, userID string, s sort.Options, f filter.Option) ([]model.TaskList, error) {
	args := m.Called(ctx, userID, s, f)
	return args.Get(0).([]model.TaskList), args.Error(1)
}
func (m *TaskRepository) FindByTag(ctx context.Context, userID, tagID string) ([]model.TaskList, error) {
	args := m.Called(ctx, userID, tagID)
	return args.Get(0).([]model.TaskList), args.Error(1)
}
func (m *TaskRepository) FindAll(ctx context.Context) ([]model.TaskList, error) {
	args := m.Called(ctx)
	return args.Get(0).([]model.TaskList), args.Error(1)
}
func (m *TaskRepository) FindAllFiltered(ctx context.Context, from, to, status, priory string) ([]model.TaskList, error) {
	args := m.Called(ctx, from, to, status, priory)
	return args.Get(0).([]model.TaskList), args.Error(1)
}
func (m *TaskRepository) AdminSoftDelete(ctx context.Context, id string) (string, error) {
	args := m.Called(ctx, id)
	return args.String(0), args.Error(1)
}
func (m *TaskRepository) AcknowledgeAdminDeletion(ctx context.Context, id, userID string) error {
	return m.Called(ctx, id, userID).Error(0)
}
func (m *TaskRepository) AdminSoftDeleteShared(ctx context.Context, id string) (string, string, string, error) {
	args := m.Called(ctx, id)
	return args.String(0), args.String(1), args.String(2), args.Error(3)
}
func (m *TaskRepository) AcknowledgeAdminDeletionShared(ctx context.Context, id, userID string) error {
	return m.Called(ctx, id, userID).Error(0)
}
func (m *TaskRepository) AdminRestore(ctx context.Context, id string) (string, error) {
	args := m.Called(ctx, id)
	return args.String(0), args.Error(1)
}
func (m *TaskRepository) AdminRestoreShared(ctx context.Context, id string) (string, string, error) {
	args := m.Called(ctx, id)
	return args.String(0), args.String(1), args.Error(2)
}

// SubtaskRepository — мок репозитория подзадач.
type SubtaskRepository struct{ mock.Mock }

func (m *SubtaskRepository) CreateSubtasks(ctx context.Context, taskID string, subtasks []domain.Subtask) error {
	return m.Called(ctx, taskID, subtasks).Error(0)
}
func (m *SubtaskRepository) CreateSubtask(ctx context.Context, taskID, ownerID, title string) (domain.Subtask, error) {
	args := m.Called(ctx, taskID, ownerID, title)
	return args.Get(0).(domain.Subtask), args.Error(1)
}
func (m *SubtaskRepository) FindByTask(ctx context.Context, taskID string) ([]domain.Subtask, error) {
	args := m.Called(ctx, taskID)
	return args.Get(0).([]domain.Subtask), args.Error(1)
}
func (m *SubtaskRepository) ToggleDone(ctx context.Context, subtaskID, ownerID string) (domain.Subtask, error) {
	args := m.Called(ctx, subtaskID, ownerID)
	return args.Get(0).(domain.Subtask), args.Error(1)
}
func (m *SubtaskRepository) AreAllDone(ctx context.Context, taskID string) (bool, error) {
	args := m.Called(ctx, taskID)
	return args.Bool(0), args.Error(1)
}
func (m *SubtaskRepository) DeleteSubtask(ctx context.Context, subtaskID, ownerID string) error {
	return m.Called(ctx, subtaskID, ownerID).Error(0)
}
func (m *SubtaskRepository) UpdateSubtask(ctx context.Context, subtaskID, ownerID, title string) error {
	return m.Called(ctx, subtaskID, ownerID, title).Error(0)
}

// CacheRepository — мок кэша.
type CacheRepository struct{ mock.Mock }

func (m *CacheRepository) SetTask(ctx context.Context, task domain.Task) error {
	return m.Called(ctx, task).Error(0)
}
func (m *CacheRepository) GetTask(ctx context.Context, id string) (domain.Task, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(domain.Task), args.Error(1)
}
func (m *CacheRepository) DeleteTask(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
func (m *CacheRepository) GetList(ctx context.Context, key string) ([]model.TaskList, error) {
	args := m.Called(ctx, key)
	return args.Get(0).([]model.TaskList), args.Error(1)
}
func (m *CacheRepository) SetList(ctx context.Context, key string, tasks []model.TaskList) error {
	return m.Called(ctx, key, tasks).Error(0)
}
func (m *CacheRepository) InvalidateUserLists(ctx context.Context, userID string) error {
	return m.Called(ctx, userID).Error(0)
}
func (m *CacheRepository) InvalidateTagTasks(ctx context.Context, tagID string) error {
	return m.Called(ctx, tagID).Error(0)
}
func (m *CacheRepository) InvalidateUserTaskCaches(ctx context.Context, userID string) error {
	return m.Called(ctx, userID).Error(0)
}
