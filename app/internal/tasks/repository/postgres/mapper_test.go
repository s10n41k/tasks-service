package postgres

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestModelToEntity_AllFields(t *testing.T) {
	tagID := "tag-uuid"
	tagName := "работа"
	now := time.Now().UTC().Round(time.Second)
	future := now.Add(24 * time.Hour)
	reminder := now.Add(-1 * time.Hour)

	m := TaskModel{
		ID:          "task-id",
		Title:       "Заголовок",
		Description: "Описание",
		Priority:    "red",
		Status:      "2",
		DueDate:     future,
		UserID:      "user-id",
		CreatedAt:   now,
		TagID:       sql.NullString{String: tagID, Valid: true},
		TagName:     sql.NullString{String: tagName, Valid: true},
		Reminder60mSentAt: sql.NullTime{Time: reminder, Valid: true},
		Reminder15mSentAt: sql.NullTime{Valid: false},
		Reminder5mSentAt:  sql.NullTime{Valid: false},
	}

	task := modelToEntity(m)

	assert.Equal(t, "task-id", task.ID)
	assert.Equal(t, "Заголовок", task.Title)
	assert.Equal(t, domain.PrioryRed, task.Priority)
	assert.Equal(t, domain.StatusInProgress, task.Status)
	assert.Equal(t, future, task.DueDate)
	assert.Equal(t, "user-id", task.UserID)
	assert.Equal(t, tagName, task.TagName)
	require := assert.New(t)
	require.NotNil(task.TagID)
	assert.Equal(t, tagID, *task.TagID)
	assert.NotNil(t, task.Reminder60mSentAt)
	assert.Nil(t, task.Reminder15mSentAt)
	assert.Nil(t, task.Reminder5mSentAt)
}

func TestModelToEntity_NullableFields(t *testing.T) {
	m := TaskModel{
		ID:      "id",
		Status:  "1",
		DueDate: time.Now().Add(time.Hour),
		TagID:   sql.NullString{Valid: false},
		TagName: sql.NullString{Valid: false},
	}
	task := modelToEntity(m)
	assert.Nil(t, task.TagID)
	assert.Empty(t, task.TagName)
}

func TestModelToEntity_OverdueTask(t *testing.T) {
	// Дедлайн прошёл + статус in_progress → должен стать not_completed
	m := TaskModel{
		ID:      "id",
		Status:  "2",
		DueDate: time.Now().Add(-time.Hour),
	}
	task := modelToEntity(m)
	assert.Equal(t, domain.StatusNotCompleted, task.Status)
}

func TestModelToEntity_CompletedOverdue(t *testing.T) {
	// Completed с прошедшим дедлайном — статус остаётся completed
	m := TaskModel{
		ID:      "id",
		Status:  "3",
		DueDate: time.Now().Add(-time.Hour),
	}
	task := modelToEntity(m)
	assert.Equal(t, domain.StatusCompleted, task.Status)
}

func TestSubtaskModelToEntity(t *testing.T) {
	m := SubtaskModel{
		ID:     "sub-id",
		TaskID: "task-id",
		Title:  "Подзадача",
		IsDone: true,
		Order:  2,
	}
	s := subtaskModelToEntity(m)
	assert.Equal(t, "sub-id", s.ID)
	assert.Equal(t, "task-id", s.TaskID)
	assert.Equal(t, "Подзадача", s.Title)
	assert.True(t, s.IsDone)
	assert.Equal(t, 2, s.Order)
}
