package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewStatus(t *testing.T) {
	cases := []struct {
		input    string
		expected Status
	}{
		{"not_completed", StatusNotCompleted},
		{"1", StatusNotCompleted},
		{"in_progress", StatusInProgress},
		{"2", StatusInProgress},
		{"completed", StatusCompleted},
		{"3", StatusCompleted},
		{"unknown", StatusInProgress},
		{"", StatusInProgress},
	}
	for _, c := range cases {
		assert.Equal(t, c.expected, NewStatus(c.input), "input=%q", c.input)
	}
}

func TestNewPriory(t *testing.T) {
	assert.Equal(t, PrioryBlue, NewPriory("blue"))
	assert.Equal(t, PrioryRed, NewPriory("red"))
	assert.Equal(t, PrioryGreen, NewPriory("green"))
	assert.Equal(t, PrioryGreen, NewPriory("unknown"))
	assert.Equal(t, PrioryGreen, NewPriory(""))
}

func TestStatusStorageCode(t *testing.T) {
	assert.Equal(t, "1", StatusNotCompleted.StorageCode())
	assert.Equal(t, "2", StatusInProgress.StorageCode())
	assert.Equal(t, "3", StatusCompleted.StorageCode())
	assert.Equal(t, "2", Status("garbage").StorageCode())
}

func TestStatusIsValid(t *testing.T) {
	assert.True(t, StatusNotCompleted.IsValid())
	assert.True(t, StatusInProgress.IsValid())
	assert.True(t, StatusCompleted.IsValid())
	assert.False(t, Status("unknown").IsValid())
	assert.False(t, Status("").IsValid())
}

func TestCanTransitionTo(t *testing.T) {
	task := &Task{Status: StatusInProgress}
	assert.True(t, task.CanTransitionTo(StatusCompleted))
	assert.True(t, task.CanTransitionTo(StatusNotCompleted))
	assert.True(t, task.CanTransitionTo(StatusInProgress))
	assert.False(t, task.CanTransitionTo(Status("invalid")))
}

func TestHasTag(t *testing.T) {
	tagID := "some-id"
	emptyID := ""

	t.Run("с TagID", func(t *testing.T) {
		task := &Task{TagID: &tagID}
		assert.True(t, task.HasTag())
	})
	t.Run("с TagName", func(t *testing.T) {
		task := &Task{TagName: "work"}
		assert.True(t, task.HasTag())
	})
	t.Run("пустой TagID", func(t *testing.T) {
		task := &Task{TagID: &emptyID}
		assert.False(t, task.HasTag())
	})
	t.Run("без тега", func(t *testing.T) {
		task := &Task{}
		assert.False(t, task.HasTag())
	})
}

func TestHasSubtasks(t *testing.T) {
	t.Run("без подзадач", func(t *testing.T) {
		task := &Task{}
		assert.False(t, task.HasSubtasks())
	})
	t.Run("с подзадачами", func(t *testing.T) {
		task := &Task{Subtasks: []Subtask{{Title: "sub"}}}
		assert.True(t, task.HasSubtasks())
	})
}

func TestAllSubtasksDone(t *testing.T) {
	t.Run("пустой список", func(t *testing.T) {
		task := &Task{}
		assert.False(t, task.AllSubtasksDone())
	})
	t.Run("все выполнены", func(t *testing.T) {
		task := &Task{Subtasks: []Subtask{
			{IsDone: true}, {IsDone: true},
		}}
		assert.True(t, task.AllSubtasksDone())
	})
	t.Run("часть выполнена", func(t *testing.T) {
		task := &Task{Subtasks: []Subtask{
			{IsDone: true}, {IsDone: false},
		}}
		assert.False(t, task.AllSubtasksDone())
	})
}

func TestPrioryIsValid(t *testing.T) {
	assert.True(t, PrioryBlue.IsValid())
	assert.True(t, PrioryRed.IsValid())
	assert.True(t, PrioryGreen.IsValid())
	assert.False(t, Priory("yellow").IsValid())
}

func TestAllSubtasksDoneNotStaleDeadline(t *testing.T) {
	// Проверяем что domain не завязан на time.Now — дедлайн не влияет на AllSubtasksDone
	past := time.Now().Add(-24 * time.Hour)
	task := &Task{
		DueDate:  past,
		Subtasks: []Subtask{{IsDone: true}},
	}
	assert.True(t, task.AllSubtasksDone())
}
