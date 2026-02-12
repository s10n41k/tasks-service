package model

import "time"

type Task struct {
	Id          string    `db:"task_id" json:"id"`
	Title       string    `db:"title" json:"title"`
	Description string    `db:"description" json:"description"`
	Priory      string    `db:"priory" json:"priory"`
	Status      string    `db:"status" json:"status"`
	DueDate     time.Time `db:"due_date" json:"due_date"`
	UserID      string    `db:"user_id" json:"user_id"`
	TagID       *string   `json:"tag_id,omitempty"`
	TagsName    string    `db:"tags_name" json:"tags_name"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}
