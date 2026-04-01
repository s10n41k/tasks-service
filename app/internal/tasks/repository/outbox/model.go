package outbox

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"encoding/json"
	"time"
)

// EventModel — персистентная модель outbox-события (только db: теги).
type EventModel struct {
	ID            string          `db:"id"`
	AggregateType string          `db:"aggregate_type"`
	AggregateID   string          `db:"aggregate_id"`
	EventType     string          `db:"event_type"`
	EventData     json.RawMessage `db:"event_data"`
	CreatedAt     time.Time       `db:"created_at"`
	ProcessedAt   *time.Time      `db:"processed_at"`
	Attempts      int             `db:"attempts"`
	LastError     *string         `db:"last_error"`
}

// TaskPayload — JSON-представление задачи для outbox/Kafka.
// Использует те же json-ключи что в кэше для совместимости.
type TaskPayload struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Priority    string    `json:"priory"`
	Status      string    `json:"status"`
	DueDate     time.Time `json:"due_date"`
	UserID      string    `json:"user_id"`
	TagID       *string   `json:"tag_id,omitempty"`
	TagName     string    `json:"tags_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// PayloadToTask конвертирует TaskPayload в доменную сущность.
func PayloadToTask(p TaskPayload) domain.Task {
	return domain.Task{
		ID:          p.ID,
		Title:       p.Title,
		Description: p.Description,
		Priority:    domain.Priory(p.Priority),
		Status:      domain.NewStatus(p.Status),
		DueDate:     p.DueDate,
		UserID:      p.UserID,
		TagID:       p.TagID,
		TagName:     p.TagName,
		CreatedAt:   p.CreatedAt,
	}
}
