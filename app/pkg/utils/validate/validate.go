package validate

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"time"
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
