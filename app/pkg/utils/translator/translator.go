package translator

import (
	model2 "TODOLIST_Tasks/app/internal/tasks/model"
	"time"
)

func Translator(num string) string {
	if num == "1" {
		return "not_completed"
	}
	if num == "2" {
		return "in_progress"
	}
	if num == "3" {
		return "completed"
	}
	return ""
}

func AntiTranslator(status string) string {
	if status == "not_completed" {
		return "1"
	}
	if status == "in_progress" {
		return "2"
	}
	if status == "completed" {
		return "3"
	}
	return ""
}

func FormatDate(date time.Time) string {
	return date.Format("02-01-2006 15:04")
}

func ToTaskResponse(task model2.Task) model2.TaskResponse {
	return model2.TaskResponse{
		Id:          task.Id,
		Title:       task.Title,
		Description: task.Description,
		Priory:      task.Priory,
		Status:      Translator(task.Status),
		DueDate:     FormatDate(task.DueDate),
		CreatedAt:   FormatDate(task.CreatedAt),
		UserId:      task.UserID,
		TagsName:    task.TagsName,
	}
}
