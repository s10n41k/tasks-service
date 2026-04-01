package port

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/model"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/api/sort"
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrDeadlineExpired = errors.New("deadline expired")
)

// TaskRepository — контракт хранилища задач.
type TaskRepository interface {
	Create(ctx context.Context, task domain.Task) error
	CreateBatch(ctx context.Context, tasks []domain.Task) error
	Update(ctx context.Context, id string, patch UpdatePatch) (domain.Task, error)
	Delete(ctx context.Context, id string) error
	DeleteBatch(ctx context.Context, ids []string) error
	FindByID(ctx context.Context, id string) (domain.Task, error)
	FindByUser(ctx context.Context, userID string, sortOpts sort.Options, filterOpts filter.Option) ([]model.TaskList, error)
	FindByTag(ctx context.Context, userID, tagID string) ([]model.TaskList, error)
	FindAll(ctx context.Context) ([]model.TaskList, error)
	FindAllFiltered(ctx context.Context, from, to, status, priory string) ([]model.TaskList, error)
	// AdminSoftDelete помечает задачу как удалённую администратором (soft-delete).
	AdminSoftDelete(ctx context.Context, id string) (string, error) // возвращает user_id владельца
	// AcknowledgeAdminDeletion физически удаляет задачу после подтверждения пользователем.
	AcknowledgeAdminDeletion(ctx context.Context, id, userID string) error
	// AdminSoftDeleteShared помечает совместную задачу как удалённую администратором.
	AdminSoftDeleteShared(ctx context.Context, id string) (proposerID, addresseeID, title string, err error)
	// AcknowledgeAdminDeletionShared физически удаляет совместную задачу после подтверждения.
	AcknowledgeAdminDeletionShared(ctx context.Context, id, userID string) error
	// AdminRestore снимает пометку удаления с задачи (admin_deleted → FALSE).
	// Возвращает user_id владельца для WS-уведомления.
	AdminRestore(ctx context.Context, id string) (string, error)
	// AdminRestoreShared снимает пометку удаления с совместной задачи.
	// Возвращает proposer_id и addressee_id для WS-уведомления.
	AdminRestoreShared(ctx context.Context, id string) (proposerID, addresseeID string, err error)
}

// SubtaskRepository — контракт хранилища подзадач обычных задач.
// Реализует бизнес-правило: ToggleDone возвращает актуальное состояние подзадачи,
// сервисный слой отвечает за проверку auto-complete агрегата Task.
type SubtaskRepository interface {
	CreateSubtasks(ctx context.Context, taskID string, subtasks []domain.Subtask) error
	// CreateSubtask добавляет одну подзадачу к существующей задаче (только владелец).
	CreateSubtask(ctx context.Context, taskID, ownerID, title string) (domain.Subtask, error)
	FindByTask(ctx context.Context, taskID string) ([]domain.Subtask, error)
	ToggleDone(ctx context.Context, subtaskID, taskOwnerID string) (domain.Subtask, error)
	AreAllDone(ctx context.Context, taskID string) (bool, error)
	DeleteSubtask(ctx context.Context, subtaskID, taskOwnerID string) error
	UpdateSubtask(ctx context.Context, subtaskID, taskOwnerID, title string) error
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
