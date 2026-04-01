package domain

import (
	"errors"
	"time"
)

// ErrNoOwnSubtask — нарушение инварианта: хотя бы 1 подзадача должна быть назначена самому proposer'у.
var ErrNoOwnSubtask = errors.New("хотя бы 1 подзадача должна быть назначена вам")

// ErrNoFriendSubtask — нарушение инварианта: хотя бы 1 подзадача должна быть назначена партнёру.
var ErrNoFriendSubtask = errors.New("хотя бы 1 подзадача должна быть назначена другу")

// SubtaskCandidate — минимальное представление подзадачи для валидации в domain.
// Не зависит от внешних пакетов (port, dto и т.д.).
type SubtaskCandidate struct {
	AssigneeID string
}

// ValidateSubtasksForProposal проверяет инварианты подзадач при создании совместной задачи:
//   - хотя бы 1 подзадача назначена proposer'у (самому себе)
//   - хотя бы 1 подзадача назначена адресату (партнёру)
func ValidateSubtasksForProposal(proposerID, addresseeID string, subtasks []SubtaskCandidate) error {
	var hasOwn, hasFriend bool
	for _, s := range subtasks {
		if s.AssigneeID == proposerID {
			hasOwn = true
		}
		if s.AssigneeID == addresseeID {
			hasFriend = true
		}
	}
	if !hasOwn {
		return ErrNoOwnSubtask
	}
	if !hasFriend {
		return ErrNoFriendSubtask
	}
	return nil
}

// SharedTaskStatus — статус совместной задачи.
type SharedTaskStatus string

const (
	StatusPending  SharedTaskStatus = "pending"
	StatusAccepted SharedTaskStatus = "accepted"
	StatusRejected SharedTaskStatus = "rejected"
)

func (s SharedTaskStatus) IsValid() bool {
	return s == StatusPending || s == StatusAccepted || s == StatusRejected
}

// SharedTask — корень агрегата совместной задачи.
// Принадлежит двум пользователям: proposer и addressee.
// Жизненный цикл: pending → accepted | rejected (переговорный процесс).
// Ключевые инварианты:
//   - proposer ≠ addressee
//   - только addressee может принять или отклонить задачу
//   - оба участника могут редактировать принятую задачу
type SharedTask struct {
	ID            string
	ProposerID    string
	AddresseeID   string
	Title         string
	Description   string
	Priority      string
	Status        SharedTaskStatus
	DueDate       *time.Time
	CreatedAt     time.Time
	ProposerName  string
	AddresseeName string
	Subtasks      []SharedSubtask
	AdminDeleted  bool
}
