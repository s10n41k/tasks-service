package postgres

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"database/sql"
	"time"
)

// modelToEntity конвертирует DB-модель в доменную сущность.
// Декодирование "1"/"2"/"3" → domain.Status происходит здесь, на границе слоёв.
func modelToEntity(m TaskModel) domain.Task {
	task := domain.Task{
		ID:          m.ID,
		Title:       m.Title,
		Description: m.Description,
		Priority:    m.Priority,
		Status:      domain.NewStatus(m.Status),
		DueDate:     m.DueDate,
		UserID:      m.UserID,
		CreatedAt:   m.CreatedAt.In(time.Local),
	}
	if m.TagID.Valid {
		s := m.TagID.String
		task.TagID = &s
	}
	if m.TagName.Valid {
		task.TagName = m.TagName.String
	}
	return task
}

// toNullString конвертирует sql.NullString в *string.
func toNullString(ns sql.NullString) *string {
	if ns.Valid {
		s := ns.String
		return &s
	}
	return nil
}
