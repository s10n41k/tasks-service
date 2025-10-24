package validate

import (
	"TODOLIST_Tasks/app/internal/tasks/model"
	"time"
)

func Validate(task model.Task) model.Task {
	switch {
	case task.Status == "":
		task.Status = "2"
	case task.Priory == "":
		task.Priory = "green"
	case task.DueDate.IsZero():
		task.DueDate = time.Now().Add(time.Hour * 24)
	}
	return task
}
