package service

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/port"
	"TODOLIST_Tasks/app/internal/tasks/port/mocks"
	logging "TODOLIST_Tasks/app/pkg/logging"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newService(t *testing.T) (*taskService, *mocks.TaskRepository, *mocks.SubtaskRepository, *mocks.CacheRepository) {
	t.Helper()
	taskRepo := &mocks.TaskRepository{}
	subtaskRepo := &mocks.SubtaskRepository{}
	cacheRepo := &mocks.CacheRepository{}
	svc := &taskService{
		tasks:    taskRepo,
		subtasks: subtaskRepo,
		cache:    cacheRepo,
		logger:   logging.GetLogger().GetLoggerWithField("service", "tasks"),
	}
	return svc, taskRepo, subtaskRepo, cacheRepo
}

// --- CreateTask ---

func TestCreateTask_Success(t *testing.T) {
	svc, taskRepo, _, _ := newService(t)
	task := domain.Task{ID: "id-1", Title: "Тест"}
	taskRepo.On("Create", mock.Anything, task).Return(nil)

	err := svc.CreateTask(context.Background(), task)
	require.NoError(t, err)
	taskRepo.AssertExpectations(t)
}

func TestCreateTask_WithSubtasks(t *testing.T) {
	svc, taskRepo, subtaskRepo, _ := newService(t)
	task := domain.Task{
		ID:       "id-2",
		Title:    "С подзадачами",
		Subtasks: []domain.Subtask{{Title: "Подзадача 1"}},
	}
	taskRepo.On("Create", mock.Anything, task).Return(nil)
	subtaskRepo.On("CreateSubtasks", mock.Anything, "id-2", task.Subtasks).Return(nil)

	err := svc.CreateTask(context.Background(), task)
	require.NoError(t, err)
	taskRepo.AssertExpectations(t)
	subtaskRepo.AssertExpectations(t)
}

func TestCreateTask_SubtasksFail(t *testing.T) {
	svc, taskRepo, subtaskRepo, _ := newService(t)
	task := domain.Task{
		ID:       "id-3",
		Subtasks: []domain.Subtask{{Title: "s"}},
	}
	taskRepo.On("Create", mock.Anything, task).Return(nil)
	subtaskRepo.On("CreateSubtasks", mock.Anything, "id-3", task.Subtasks).
		Return(errors.New("db error"))

	err := svc.CreateTask(context.Background(), task)
	assert.Error(t, err)
}

// --- UpdateTask ---

func TestUpdateTask_ValidTransition(t *testing.T) {
	svc, taskRepo, _, _ := newService(t)
	status := domain.StatusCompleted
	patch := port.UpdatePatch{Status: &status}
	current := domain.Task{ID: "id-1", Status: domain.StatusInProgress}
	updated := domain.Task{ID: "id-1", Status: domain.StatusCompleted}

	taskRepo.On("FindByID", mock.Anything, "id-1").Return(current, nil)
	taskRepo.On("Update", mock.Anything, "id-1", patch).Return(updated, nil)

	result, err := svc.UpdateTask(context.Background(), "id-1", patch)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusCompleted, result.Status)
	taskRepo.AssertExpectations(t)
}

func TestUpdateTask_InvalidTransition(t *testing.T) {
	svc, taskRepo, _, _ := newService(t)
	badStatus := domain.Status("invalid")
	patch := port.UpdatePatch{Status: &badStatus}
	current := domain.Task{ID: "id-1", Status: domain.StatusInProgress}

	taskRepo.On("FindByID", mock.Anything, "id-1").Return(current, nil)

	_, err := svc.UpdateTask(context.Background(), "id-1", patch)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "недопустимый переход статуса")
}

func TestUpdateTask_NoStatus(t *testing.T) {
	svc, taskRepo, _, _ := newService(t)
	// Патч без статуса — FindByID не должен вызываться
	title := "Новый заголовок"
	patch := port.UpdatePatch{Title: &title}
	updated := domain.Task{ID: "id-1", Title: "Новый заголовок"}
	taskRepo.On("Update", mock.Anything, "id-1", patch).Return(updated, nil)

	result, err := svc.UpdateTask(context.Background(), "id-1", patch)
	require.NoError(t, err)
	assert.Equal(t, "Новый заголовок", result.Title)
	taskRepo.AssertNotCalled(t, "FindByID")
}

// --- DeleteTask ---

func TestDeleteTask_Success(t *testing.T) {
	svc, taskRepo, _, _ := newService(t)
	taskRepo.On("Delete", mock.Anything, "id-1").Return(nil)

	err := svc.DeleteTask(context.Background(), "id-1")
	require.NoError(t, err)
	taskRepo.AssertExpectations(t)
}

// --- FindTask ---

func TestFindTask_CacheHit(t *testing.T) {
	svc, _, subtaskRepo, cacheRepo := newService(t)
	cached := domain.Task{ID: "id-1", Title: "Из кэша"}
	subtasks := []domain.Subtask{{Title: "s1"}}

	cacheRepo.On("GetTask", mock.Anything, "id-1").Return(cached, nil)
	subtaskRepo.On("FindByTask", mock.Anything, "id-1").Return(subtasks, nil)

	task, fromCache, err := svc.FindTask(context.Background(), "id-1")
	require.NoError(t, err)
	assert.True(t, fromCache)
	assert.Equal(t, "Из кэша", task.Title)
	assert.Equal(t, subtasks, task.Subtasks)
}

func TestFindTask_CacheMiss(t *testing.T) {
	svc, taskRepo, _, cacheRepo := newService(t)
	dbTask := domain.Task{ID: "id-1", Title: "Из БД"}

	cacheRepo.On("GetTask", mock.Anything, "id-1").Return(domain.Task{}, errors.New("miss"))
	taskRepo.On("FindByID", mock.Anything, "id-1").Return(dbTask, nil)

	task, fromCache, err := svc.FindTask(context.Background(), "id-1")
	require.NoError(t, err)
	assert.False(t, fromCache)
	assert.Equal(t, "Из БД", task.Title)
}

// --- ToggleSubtaskDone ---

func TestToggleSubtaskDone_AutoComplete(t *testing.T) {
	svc, taskRepo, subtaskRepo, _ := newService(t)
	subtask := domain.Subtask{ID: "s-1", TaskID: "t-1", IsDone: true}
	completedStatus := domain.StatusCompleted

	subtaskRepo.On("ToggleDone", mock.Anything, "s-1", "owner").Return(subtask, nil)
	subtaskRepo.On("AreAllDone", mock.Anything, "t-1").Return(true, nil)
	taskRepo.On("Update", mock.Anything, "t-1", port.UpdatePatch{Status: &completedStatus}).
		Return(domain.Task{}, nil)

	result, err := svc.ToggleSubtaskDone(context.Background(), "s-1", "owner")
	require.NoError(t, err)
	assert.True(t, result.IsDone)
	taskRepo.AssertExpectations(t)
}

func TestToggleSubtaskDone_NotAllDone(t *testing.T) {
	svc, taskRepo, subtaskRepo, _ := newService(t)
	subtask := domain.Subtask{ID: "s-1", TaskID: "t-1", IsDone: true}

	subtaskRepo.On("ToggleDone", mock.Anything, "s-1", "owner").Return(subtask, nil)
	subtaskRepo.On("AreAllDone", mock.Anything, "t-1").Return(false, nil)

	_, err := svc.ToggleSubtaskDone(context.Background(), "s-1", "owner")
	require.NoError(t, err)
	taskRepo.AssertNotCalled(t, "Update")
}

func TestToggleSubtaskDone_Uncheck(t *testing.T) {
	svc, taskRepo, subtaskRepo, _ := newService(t)
	// IsDone=false → AreAllDone не вызывается, Update не вызывается
	subtask := domain.Subtask{ID: "s-1", TaskID: "t-1", IsDone: false}
	subtaskRepo.On("ToggleDone", mock.Anything, "s-1", "owner").Return(subtask, nil)

	_, err := svc.ToggleSubtaskDone(context.Background(), "s-1", "owner")
	require.NoError(t, err)
	subtaskRepo.AssertNotCalled(t, "AreAllDone")
	taskRepo.AssertNotCalled(t, "Update")
}
