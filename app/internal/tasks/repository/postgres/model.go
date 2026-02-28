package postgres

import (
	"database/sql"
	"time"
)

// TaskModel — персистентная модель. Только db: теги, без бизнес-логики.
type TaskModel struct {
	ID          string         `db:"task_id"`
	Title       string         `db:"title"`
	Description string         `db:"description"`
	Priority    string         `db:"priory"`
	Status      string         `db:"status"` // хранится как "1"/"2"/"3"
	DueDate     time.Time      `db:"due_date"`
	UserID      string         `db:"user_id"`
	TagID       sql.NullString `db:"tag_id"`
	TagName     sql.NullString `db:"tags_name"`
	CreatedAt   time.Time      `db:"created_at"`
}
