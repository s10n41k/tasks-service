package domain

import "time"

type Status string

const (
	StatusNotCompleted Status = "not_completed"
	StatusInProgress   Status = "in_progress"
	StatusCompleted    Status = "completed"
)

type Priory string

const (
	PrioryBlue  Priory = "blue"
	PrioryRed   Priory = "red"
	PrioryGreen Priory = "green"
)

// Task — корень агрегата задачи. Без db/json тегов.
// Содержит Subtasks как часть агрегата (не отдельный домен).
type Task struct {
	ID          string
	Title       string
	Description string
	Priority    Priory
	Status      Status
	DueDate     time.Time
	UserID      string
	TagID       *string
	TagName     string
	CreatedAt   time.Time
	Subtasks    []Subtask
	// Поля для напоминаний — заполняются только воркером, не входят в API ответы.
	Reminder60mSentAt *time.Time
	Reminder15mSentAt *time.Time
	Reminder5mSentAt  *time.Time
}

func NewPriory(s string) Priory {
	switch s {
	case "blue":
		return PrioryBlue
	case "red":
		return PrioryRed
	case "green":
		return PrioryGreen
	default:
		return PrioryGreen
	}
}

func (p Priory) IsValid() bool {
	return p == PrioryBlue || p == PrioryGreen || p == PrioryRed
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

// HasSubtasks возвращает true если у задачи есть подзадачи.
func (t *Task) HasSubtasks() bool {
	return len(t.Subtasks) > 0
}

// AllSubtasksDone возвращает true если у задачи есть подзадачи и все выполнены.
// Если подзадач нет — возвращает false (задача управляется вручную).
func (t *Task) AllSubtasksDone() bool {
	if len(t.Subtasks) == 0 {
		return false
	}
	for _, s := range t.Subtasks {
		if !s.IsDone {
			return false
		}
	}
	return true
}
