package service

import (
	"TODOLIST_Tasks/app/internal/shared_tasks/domain"
	"TODOLIST_Tasks/app/internal/shared_tasks/port"
	logging "TODOLIST_Tasks/app/pkg/logging"
	"context"
	"fmt"
)

// SharedTaskService — сервисный слой для совместных задач.
// Оркестрирует бизнес-логику агрегата SharedTask.
type SharedTaskService interface {
	Propose(ctx context.Context, proposerID, addresseeID, title, desc, priority, dueDate string, subtasks []port.SubtaskInput) (string, error)
	FindByUser(ctx context.Context, userID string) ([]domain.SharedTask, error)
	AdminFindAll(ctx context.Context) ([]domain.SharedTask, error)
	FindByID(ctx context.Context, taskID, userID string) (*domain.SharedTask, error)
	Accept(ctx context.Context, taskID, userID string) error
	Reject(ctx context.Context, taskID, userID string) error
	// Update возвращает wasCounterPropose=true если адресат сделал встречное предложение.
	Update(ctx context.Context, taskID, userID string, input port.UpdateInput) (wasCounterPropose bool, err error)
	Delete(ctx context.Context, taskID, userID string) (partnerID string, taskTitle string, err error)
	ToggleSubtaskDone(ctx context.Context, subtaskID, assigneeID string) (domain.SharedSubtask, error)
}

type sharedTaskService struct {
	repo   port.SharedTaskRepository
	logger *logging.Logger
}

func New(repo port.SharedTaskRepository) SharedTaskService {
	return &sharedTaskService{
		repo:   repo,
		logger: logging.GetLogger().GetLoggerWithField("service", "shared_tasks"),
	}
}

func (s *sharedTaskService) Propose(ctx context.Context, proposerID, addresseeID, title, desc, priority, dueDate string, subtasks []port.SubtaskInput) (string, error) {
	if proposerID == addresseeID {
		return "", fmt.Errorf("нельзя создать совместную задачу с самим собой")
	}

	// Фильтруем пустые подзадачи и конвертируем в domain-тип для валидации инварианта.
	candidates := make([]domain.SubtaskCandidate, 0, len(subtasks))
	for _, s := range subtasks {
		if s.Title != "" {
			candidates = append(candidates, domain.SubtaskCandidate{AssigneeID: s.AssigneeID})
		}
	}
	if err := domain.ValidateSubtasksForProposal(proposerID, addresseeID, candidates); err != nil {
		return "", err
	}

	id, err := s.repo.Propose(ctx, proposerID, addresseeID, title, desc, priority, dueDate, subtasks)
	if err != nil {
		s.logger.Errorf("Propose: proposer %s addressee %s: %v", proposerID, addresseeID, err)
		return "", err
	}
	return id, nil
}

func (s *sharedTaskService) FindByUser(ctx context.Context, userID string) ([]domain.SharedTask, error) {
	tasks, err := s.repo.FindByUser(ctx, userID)
	if err != nil {
		s.logger.Errorf("FindByUser: user %s: %v", userID, err)
		return nil, err
	}
	return tasks, nil
}

func (s *sharedTaskService) AdminFindAll(ctx context.Context) ([]domain.SharedTask, error) {
	tasks, err := s.repo.FindAll(ctx)
	if err != nil {
		s.logger.Errorf("AdminFindAll: %v", err)
		return nil, err
	}
	return tasks, nil
}

func (s *sharedTaskService) FindByID(ctx context.Context, taskID, userID string) (*domain.SharedTask, error) {
	task, err := s.repo.FindByID(ctx, taskID, userID)
	if err != nil {
		s.logger.Errorf("FindByID: task %s user %s: %v", taskID, userID, err)
		return nil, err
	}
	return task, nil
}

func (s *sharedTaskService) Accept(ctx context.Context, taskID, userID string) error {
	if err := s.repo.Respond(ctx, taskID, userID, string(domain.StatusAccepted)); err != nil {
		s.logger.Errorf("Accept: task %s user %s: %v", taskID, userID, err)
		return err
	}
	return nil
}

func (s *sharedTaskService) Reject(ctx context.Context, taskID, userID string) error {
	if err := s.repo.Respond(ctx, taskID, userID, string(domain.StatusRejected)); err != nil {
		s.logger.Errorf("Reject: task %s user %s: %v", taskID, userID, err)
		return err
	}
	return nil
}

func (s *sharedTaskService) Update(ctx context.Context, taskID, userID string, input port.UpdateInput) (bool, error) {
	task, err := s.repo.FindByID(ctx, taskID, userID)
	if err != nil {
		s.logger.Errorf("Update FindByID: task %s user %s: %v", taskID, userID, err)
		return false, err
	}
	if task == nil {
		return false, fmt.Errorf("задача не найдена")
	}
	// Адресат редактирует ожидающую задачу → встречное предложение (меняем роли)
	if task.Status == domain.StatusPending && task.AddresseeID == userID {
		if err := s.repo.CounterPropose(ctx, taskID, userID, input); err != nil {
			s.logger.Errorf("CounterPropose: task %s user %s: %v", taskID, userID, err)
			return false, err
		}
		return true, nil
	}
	if err := s.repo.Update(ctx, taskID, userID, input); err != nil {
		s.logger.Errorf("Update: task %s user %s: %v", taskID, userID, err)
		return false, err
	}
	return false, nil
}

func (s *sharedTaskService) Delete(ctx context.Context, taskID, userID string) (string, string, error) {
	partnerID, title, err := s.repo.Delete(ctx, taskID, userID)
	if err != nil {
		s.logger.Errorf("Delete: task %s user %s: %v", taskID, userID, err)
		return "", "", err
	}
	return partnerID, title, nil
}

func (s *sharedTaskService) ToggleSubtaskDone(ctx context.Context, subtaskID, assigneeID string) (domain.SharedSubtask, error) {
	subtask, err := s.repo.ToggleSubtaskDone(ctx, subtaskID, assigneeID)
	if err != nil {
		s.logger.Errorf("ToggleSubtaskDone: subtask %s assignee %s: %v", subtaskID, assigneeID, err)
		return domain.SharedSubtask{}, err
	}
	return subtask, nil
}
