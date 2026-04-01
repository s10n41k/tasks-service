package dto

import (
	"TODOLIST_Tasks/app/internal/shared_tasks/domain"
	"time"
)

// SharedSubtaskResponse — представление подзадачи совместной задачи для HTTP-ответа.
type SharedSubtaskResponse struct {
	ID           string `json:"id"`
	SharedTaskID string `json:"shared_task_id"`
	Title        string `json:"title"`
	AssigneeID   string `json:"assignee_id"`
	AssigneeName string `json:"assignee_name"`
	IsDone       bool   `json:"is_done"`
	Order        int    `json:"order_num"`
}

// SharedTaskResponse — представление совместной задачи для HTTP-ответа.
type SharedTaskResponse struct {
	ID            string                  `json:"id"`
	ProposerID    string                  `json:"proposer_id"`
	AddresseeID   string                  `json:"addressee_id"`
	Title         string                  `json:"title"`
	Description   string                  `json:"description"`
	Priority      string                  `json:"priority"`
	Status        string                  `json:"status"`
	DueDate       *time.Time              `json:"due_date"`
	CreatedAt     time.Time               `json:"created_at"`
	ProposerName  string                  `json:"proposer_name"`
	AddresseeName string                  `json:"addressee_name"`
	Subtasks      []SharedSubtaskResponse `json:"subtasks"`
	AdminDeleted  bool                    `json:"admin_deleted,omitempty"`
}

// ToResponse маппит доменную сущность в HTTP-ответ.
func ToResponse(t domain.SharedTask) SharedTaskResponse {
	resp := SharedTaskResponse{
		ID:            t.ID,
		ProposerID:    t.ProposerID,
		AddresseeID:   t.AddresseeID,
		Title:         t.Title,
		Description:   t.Description,
		Priority:      t.Priority,
		Status:        string(t.Status),
		DueDate:       t.DueDate,
		CreatedAt:     t.CreatedAt,
		ProposerName:  t.ProposerName,
		AddresseeName: t.AddresseeName,
		AdminDeleted:  t.AdminDeleted,
		Subtasks:      make([]SharedSubtaskResponse, 0, len(t.Subtasks)),
	}
	for _, s := range t.Subtasks {
		resp.Subtasks = append(resp.Subtasks, SharedSubtaskResponse{
			ID:           s.ID,
			SharedTaskID: s.SharedTaskID,
			Title:        s.Title,
			AssigneeID:   s.AssigneeID,
			AssigneeName: s.AssigneeName,
			IsDone:       s.IsDone,
			Order:        s.Order,
		})
	}
	return resp
}

// ToResponses маппит слайс доменных сущностей в слайс HTTP-ответов.
func ToResponses(tasks []domain.SharedTask) []SharedTaskResponse {
	resp := make([]SharedTaskResponse, len(tasks))
	for i, t := range tasks {
		resp[i] = ToResponse(t)
	}
	return resp
}
