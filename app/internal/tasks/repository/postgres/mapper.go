package postgres

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"database/sql"
	"time"
)

// modelToEntity конвертирует DB-модель задачи в доменную сущность.
// Декодирование "1"/"2"/"3" → domain.Status происходит здесь, на границе слоёв.
func modelToEntity(m TaskModel) domain.Task {
	task := domain.Task{
		ID:          m.ID,
		Title:       m.Title,
		Description: m.Description,
		Priority:    domain.Priory(m.Priority),
		Status:      domain.NewStatus(m.Status),
		DueDate:     m.DueDate,
		UserID:      m.UserID,
		CreatedAt:   m.CreatedAt.In(time.Local),
	}
	// Вычисляемый статус: если дедлайн прошёл и задача не завершена → not_completed
	if task.Status != domain.StatusCompleted && !task.DueDate.IsZero() && task.DueDate.Before(time.Now()) {
		task.Status = domain.StatusNotCompleted
	}
	if m.TagID.Valid {
		s := m.TagID.String
		task.TagID = &s
	}
	if m.TagName.Valid {
		task.TagName = m.TagName.String
	}
	if m.Reminder60mSentAt.Valid {
		t := m.Reminder60mSentAt.Time
		task.Reminder60mSentAt = &t
	}
	if m.Reminder15mSentAt.Valid {
		t := m.Reminder15mSentAt.Time
		task.Reminder15mSentAt = &t
	}
	if m.Reminder5mSentAt.Valid {
		t := m.Reminder5mSentAt.Time
		task.Reminder5mSentAt = &t
	}
	return task
}

// subtaskModelToEntity конвертирует DB-модель подзадачи в доменную сущность.
func subtaskModelToEntity(m SubtaskModel) domain.Subtask {
	return domain.Subtask{
		ID:     m.ID,
		TaskID: m.TaskID,
		Title:  m.Title,
		IsDone: m.IsDone,
		Order:  m.Order,
	}
}

// toNullString конвертирует sql.NullString в *string.
func toNullString(ns sql.NullString) *string {
	if ns.Valid {
		s := ns.String
		return &s
	}
	return nil
}
