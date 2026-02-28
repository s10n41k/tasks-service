package domain

import "time"

type Status string

const (
	StatusNotCompleted Status = "not_completed"
	StatusInProgress   Status = "in_progress"
	StatusCompleted    Status = "completed"
)

// Task — доменная сущность задачи. Без db/json тегов.
type Task struct {
	ID          string
	Title       string
	Description string
	Priority    string
	Status      Status
	DueDate     time.Time
	UserID      string
	TagID       *string
	TagName     string
	CreatedAt   time.Time
}

// NewStatus разбирает строку статуса в любом формате ("1"/"not_completed").
// Неизвестный формат → StatusInProgress (текущее дефолтное поведение).
func NewStatus(s string) Status {
	switch s {
	case "not_completed", "1":
		return StatusNotCompleted
	case "in_progress", "2":
		return StatusInProgress
	case "completed", "3":
		return StatusCompleted
	default:
		return StatusInProgress
	}
}

// StorageCode возвращает числовой код для хранения в SMALLINT колонке БД.
func (s Status) StorageCode() string {
	switch s {
	case StatusNotCompleted:
		return "1"
	case StatusInProgress:
		return "2"
	case StatusCompleted:
		return "3"
	default:
		return "2"
	}
}

// IsValid проверяет допустимость значения статуса.
func (s Status) IsValid() bool {
	return s == StatusNotCompleted || s == StatusInProgress || s == StatusCompleted
}

// CanTransitionTo проверяет допустимость смены статуса.
func (t *Task) CanTransitionTo(newStatus Status) bool {
	return newStatus.IsValid()
}

// HasTag возвращает true если у задачи назначен тег.
func (t *Task) HasTag() bool {
	return (t.TagID != nil && *t.TagID != "") || t.TagName != ""
}