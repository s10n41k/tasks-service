package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTaskRequest_UnmarshalJSON(t *testing.T) {
	t.Run("валидный запрос", func(t *testing.T) {
		raw := `{
			"title": "Купить молоко",
			"description": "2%",
			"priory": "red",
			"status": "in_progress",
			"due_date": "2026-12-31 18:00",
			"tag_name": "покупки",
			"subtasks": [{"title": "В магазин"}]
		}`
		var req CreateTaskRequest
		require.NoError(t, json.Unmarshal([]byte(raw), &req))

		assert.Equal(t, "Купить молоко", req.Title)
		assert.Equal(t, "red", req.Priority)
		assert.Equal(t, "in_progress", req.Status)
		assert.Equal(t, "покупки", req.TagName)
		assert.Len(t, req.Subtasks, 1)
		assert.Equal(t, "В магазин", req.Subtasks[0].Title)

		expected, _ := time.Parse("2006-01-02 15:04", "2026-12-31 18:00")
		assert.Equal(t, expected, req.DueDate)
	})

	t.Run("невалидный due_date", func(t *testing.T) {
		raw := `{"title":"t","due_date":"not-a-date"}`
		var req CreateTaskRequest
		err := json.Unmarshal([]byte(raw), &req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid due_date format")
	})

	t.Run("с tag_id", func(t *testing.T) {
		tagID := "abc-123"
		raw := `{"title":"t","due_date":"2026-01-01 00:00","tag_id":"abc-123"}`
		var req CreateTaskRequest
		require.NoError(t, json.Unmarshal([]byte(raw), &req))
		require.NotNil(t, req.TagID)
		assert.Equal(t, tagID, *req.TagID)
	})
}

func TestCustomTime_UnmarshalJSON(t *testing.T) {
	t.Run("корректный формат", func(t *testing.T) {
		raw := `"2026-06-15 10:30"`
		var ct CustomTime
		require.NoError(t, json.Unmarshal([]byte(raw), &ct))
		expected, _ := time.Parse("2006-01-02 15:04", "2026-06-15 10:30")
		assert.Equal(t, expected, time.Time(ct))
	})

	t.Run("некорректный формат", func(t *testing.T) {
		raw := `"15/06/2026"`
		var ct CustomTime
		err := json.Unmarshal([]byte(raw), &ct)
		assert.Error(t, err)
	})
}
