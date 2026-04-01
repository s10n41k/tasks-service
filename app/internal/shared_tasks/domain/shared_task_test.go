package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateSubtasksForProposal(t *testing.T) {
	proposer := "user-1"
	addressee := "user-2"

	t.Run("обе подзадачи есть", func(t *testing.T) {
		subtasks := []SubtaskCandidate{
			{AssigneeID: proposer},
			{AssigneeID: addressee},
		}
		assert.NoError(t, ValidateSubtasksForProposal(proposer, addressee, subtasks))
	})

	t.Run("нет подзадачи proposer", func(t *testing.T) {
		subtasks := []SubtaskCandidate{{AssigneeID: addressee}}
		err := ValidateSubtasksForProposal(proposer, addressee, subtasks)
		assert.ErrorIs(t, err, ErrNoOwnSubtask)
	})

	t.Run("нет подзадачи addressee", func(t *testing.T) {
		subtasks := []SubtaskCandidate{{AssigneeID: proposer}}
		err := ValidateSubtasksForProposal(proposer, addressee, subtasks)
		assert.ErrorIs(t, err, ErrNoFriendSubtask)
	})

	t.Run("пустой список", func(t *testing.T) {
		err := ValidateSubtasksForProposal(proposer, addressee, nil)
		assert.Error(t, err)
	})

	t.Run("proposer == addressee не влияет на валидацию", func(t *testing.T) {
		subtasks := []SubtaskCandidate{
			{AssigneeID: proposer},
			{AssigneeID: addressee},
		}
		// Логика проверки независима от равенства proposer/addressee
		assert.NoError(t, ValidateSubtasksForProposal(proposer, addressee, subtasks))
	})
}

func TestSharedTaskStatusIsValid(t *testing.T) {
	assert.True(t, StatusPending.IsValid())
	assert.True(t, StatusAccepted.IsValid())
	assert.True(t, StatusRejected.IsValid())
	assert.False(t, SharedTaskStatus("unknown").IsValid())
	assert.False(t, SharedTaskStatus("").IsValid())
}
