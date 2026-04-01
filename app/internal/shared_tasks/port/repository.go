package port

import (
	"TODOLIST_Tasks/app/internal/shared_tasks/domain"
	"context"
)

// SubtaskInput — подзадача при создании/редактировании совместной задачи.
type SubtaskInput struct {
	Title      string `json:"title"`
	AssigneeID string `json:"assignee_id"`
}

// UpdateInput — поля для обновления совместной задачи.
type UpdateInput struct {
	Title       string
	Description string
	Priority    string
	DueDate     string         // ISO строка; "" = убрать дедлайн
	Subtasks    *[]SubtaskInput // nil = не трогать подзадачи, [] = удалить все, [...] = перезаписать
}

// SharedTaskRepository — контракт хранилища совместных задач.
type SharedTaskRepository interface {
	Propose(ctx context.Context, proposerID, addresseeID, title, desc, priority, dueDate string, subtasks []SubtaskInput) (string, error)
	FindByUser(ctx context.Context, userID string) ([]domain.SharedTask, error)
	FindAll(ctx context.Context) ([]domain.SharedTask, error)
	FindByID(ctx context.Context, taskID, userID string) (*domain.SharedTask, error)
	Respond(ctx context.Context, taskID, userID, status string) error
	Update(ctx context.Context, taskID, userID string, input UpdateInput) error
	// CounterPropose меняет поля задачи и меняет proposer/addressee местами (ответное предложение адресата).
	CounterPropose(ctx context.Context, taskID, addresseeID string, input UpdateInput) error
	Delete(ctx context.Context, taskID, userID string) (partnerID string, taskTitle string, err error)
	ToggleSubtaskDone(ctx context.Context, subtaskID, assigneeID string) (domain.SharedSubtask, error)
}
