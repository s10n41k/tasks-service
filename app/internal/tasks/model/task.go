package model

import "time"

// TaskList read-сущность для task
type TaskList struct {
	ID             string     `json:"task_id"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Status         string     `json:"status"`
	Priory         string     `json:"priory"`
	DueDate        time.Time  `json:"due_date"`
	UserID         string     `json:"user_id"`
	TagID          *string    `json:"tag_id,omitempty"`
	TagName        *string    `json:"tag_name,omitempty"`
	AdminDeleted   bool       `json:"admin_deleted,omitempty"`
	AdminDeletedAt *time.Time `json:"admin_deleted_at,omitempty"`
}
