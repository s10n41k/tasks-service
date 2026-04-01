package dto

import (
	"encoding/json"
	"fmt"
	"time"
)

// SubtaskInput — подзадача при создании задачи.
type SubtaskInput struct {
	Title string `json:"title"`
}

// CreateTaskRequest — входные данные создания задачи (только json: теги).
type CreateTaskRequest struct {
	Title       string
	Description string
	Priority    string
	Status      string
	DueDate     time.Time
	TagID       *string
	TagName     string
	Subtasks    []SubtaskInput
}

func (r *CreateTaskRequest) UnmarshalJSON(data []byte) error {
	var a struct {
		Title       string         `json:"title"`
		Description string         `json:"description"`
		Priority    string         `json:"priory"`
		Status      string         `json:"status"`
		DueDate     string         `json:"due_date"`
		TagID       *string        `json:"tag_id,omitempty"`
		TagName     string         `json:"tag_name"`
		Subtasks    []SubtaskInput `json:"subtasks"`
	}
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	t, err := time.Parse("2006-01-02 15:04", a.DueDate)
	if err != nil {
		return fmt.Errorf("invalid due_date format: %w", err)
	}
	r.Title = a.Title
	r.Description = a.Description
	r.Priority = a.Priority
	r.Status = a.Status
	r.DueDate = t
	r.TagID = a.TagID
	r.TagName = a.TagName
	r.Subtasks = a.Subtasks
	return nil
}

// UpdateTaskRequest — патч-обновление задачи (все поля опциональны).
type UpdateTaskRequest struct {
	Title       *string     `json:"title"`
	Description *string     `json:"description"`
	Status      *string     `json:"status"`
	Priority    *string     `json:"priory"`
	DueDate     *CustomTime `json:"due_date"`
	TagName     *string     `json:"tag_name"`
}

// SubtaskResponse — подзадача в HTTP-ответе.
type SubtaskResponse struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	IsDone bool   `json:"is_done"`
	Order  int    `json:"order"`
}

// TaskResponse — HTTP-ответ по задаче (форматированные даты).
type TaskResponse struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Priority    string            `json:"priory"`
	Status      string            `json:"status"`
	DueDate     string            `json:"due_date"`
	CreatedAt   string            `json:"created_at"`
	UserID      string            `json:"user_id"`
	TagName     string            `json:"tags_name"`
	Subtasks    []SubtaskResponse `json:"subtasks,omitempty"`
}

// CustomTime десериализует формат "2006-01-02 15:04".
type CustomTime time.Time

func (ct *CustomTime) UnmarshalJSON(b []byte) error {
	s := string(b[1 : len(b)-1])
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		return fmt.Errorf("failed to parse %q: %w", s, err)
	}
	*ct = CustomTime(t)
	return nil
}
