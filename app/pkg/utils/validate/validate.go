package validate

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func Validate(task domain.Task) domain.Task {
	switch {
	case task.Status == "":
		task.Status = domain.StatusInProgress
	case task.Priority == "":
		task.Priority = "green"
	case task.DueDate.IsZero():
		task.DueDate = time.Now().Add(time.Hour * 24)
	}
	return task
}

// UUID проверяет корректность UUID-строки.
func UUID(id string) error {
	if id == "" {
		return fmt.Errorf("id не может быть пустым")
	}
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("невалидный UUID: %s", id)
	}
	return nil
}
