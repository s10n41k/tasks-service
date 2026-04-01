package service

import (
	"TODOLIST_Tasks/app/internal/shared_tasks/domain"
	"TODOLIST_Tasks/app/internal/shared_tasks/port"
	logging "TODOLIST_Tasks/app/pkg/logging"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockSharedTaskRepo — мок репозитория совместных задач.
type mockSharedTaskRepo struct{ mock.Mock }

func (m *mockSharedTaskRepo) Propose(ctx context.Context, proposerID, addresseeID, title, desc, priority, dueDate string, subtasks []port.SubtaskInput) (string, error) {
	args := m.Called(ctx, proposerID, addresseeID, title, desc, priority, dueDate, subtasks)
	return args.String(0), args.Error(1)
}
func (m *mockSharedTaskRepo) FindByUser(ctx context.Context, userID string) ([]domain.SharedTask, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]domain.SharedTask), args.Error(1)
}
func (m *mockSharedTaskRepo) FindAll(ctx context.Context) ([]domain.SharedTask, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.SharedTask), args.Error(1)
}
func (m *mockSharedTaskRepo) FindByID(ctx context.Context, taskID, userID string) (*domain.SharedTask, error) {
	args := m.Called(ctx, taskID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SharedTask), args.Error(1)
}
func (m *mockSharedTaskRepo) Respond(ctx context.Context, taskID, userID, status string) error {
	return m.Called(ctx, taskID, userID, status).Error(0)
}
func (m *mockSharedTaskRepo) Update(ctx context.Context, taskID, userID string, input port.UpdateInput) error {
	return m.Called(ctx, taskID, userID, input).Error(0)
}
func (m *mockSharedTaskRepo) CounterPropose(ctx context.Context, taskID, addresseeID string, input port.UpdateInput) error {
	return m.Called(ctx, taskID, addresseeID, input).Error(0)
}
func (m *mockSharedTaskRepo) Delete(ctx context.Context, taskID, userID string) (string, string, error) {
	args := m.Called(ctx, taskID, userID)
	return args.String(0), args.String(1), args.Error(2)
}
func (m *mockSharedTaskRepo) ToggleSubtaskDone(ctx context.Context, subtaskID, assigneeID string) (domain.SharedSubtask, error) {
	args := m.Called(ctx, subtaskID, assigneeID)
	return args.Get(0).(domain.SharedSubtask), args.Error(1)
}

func newSharedService(t *testing.T) (*sharedTaskService, *mockSharedTaskRepo) {
	t.Helper()
	repo := &mockSharedTaskRepo{}
	svc := &sharedTaskService{
		repo:   repo,
		logger: logging.GetLogger().GetLoggerWithField("service", "shared_tasks"),
	}
	return svc, repo
}

// --- Propose ---

func TestPropose_SameUser(t *testing.T) {
	svc, _ := newSharedService(t)
	_, err := svc.Propose(context.Background(), "user-1", "user-1", "title", "", "red", "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "самим собой")
}

func TestPropose_NoOwnSubtask(t *testing.T) {
	svc, _ := newSharedService(t)
	// Нет подзадачи, назначенной самому proposer'у
	subtasks := []port.SubtaskInput{{Title: "s1", AssigneeID: "user-2"}}
	_, err := svc.Propose(context.Background(), "user-1", "user-2", "title", "", "red", "", subtasks)
	assert.ErrorIs(t, err, domain.ErrNoOwnSubtask)
}

func TestPropose_Success(t *testing.T) {
	svc, repo := newSharedService(t)
	subtasks := []port.SubtaskInput{
		{Title: "Моя задача", AssigneeID: "user-1"},
		{Title: "Твоя задача", AssigneeID: "user-2"},
	}
	repo.On("Propose", mock.Anything, "user-1", "user-2", "Совместная", "", "red", "", subtasks).
		Return("new-id", nil)

	id, err := svc.Propose(context.Background(), "user-1", "user-2", "Совместная", "", "red", "", subtasks)
	require.NoError(t, err)
	assert.Equal(t, "new-id", id)
	repo.AssertExpectations(t)
}

// --- Update ---

func TestUpdate_CounterPropose(t *testing.T) {
	svc, repo := newSharedService(t)
	// Адресат редактирует ожидающую задачу → должно быть CounterPropose
	task := &domain.SharedTask{
		ID:          "t-1",
		ProposerID:  "user-1",
		AddresseeID: "user-2",
		Status:      domain.StatusPending,
	}
	input := port.UpdateInput{Title: "Встречное предложение"}

	repo.On("FindByID", mock.Anything, "t-1", "user-2").Return(task, nil)
	repo.On("CounterPropose", mock.Anything, "t-1", "user-2", input).Return(nil)

	wasCounter, err := svc.Update(context.Background(), "t-1", "user-2", input)
	require.NoError(t, err)
	assert.True(t, wasCounter)
	repo.AssertNotCalled(t, "Update")
	repo.AssertExpectations(t)
}

func TestUpdate_Normal(t *testing.T) {
	svc, repo := newSharedService(t)
	// Proposer редактирует принятую задачу → обычный Update
	task := &domain.SharedTask{
		ID:          "t-1",
		ProposerID:  "user-1",
		AddresseeID: "user-2",
		Status:      domain.StatusAccepted,
	}
	input := port.UpdateInput{Title: "Обновлённое название"}

	repo.On("FindByID", mock.Anything, "t-1", "user-1").Return(task, nil)
	repo.On("Update", mock.Anything, "t-1", "user-1", input).Return(nil)

	wasCounter, err := svc.Update(context.Background(), "t-1", "user-1", input)
	require.NoError(t, err)
	assert.False(t, wasCounter)
	repo.AssertNotCalled(t, "CounterPropose")
	repo.AssertExpectations(t)
}
