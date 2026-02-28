package service

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/port"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/api/sort"
	logging2 "TODOLIST_Tasks/app/pkg/logging"
	"context"
	"fmt"
)

type taskService struct {
	tasks  port.TaskRepository
	cache  port.CacheRepository
	logger *logging2.Logger
}

// New создаёт сервис и возвращает три интерфейса через один объект.
func New(tasks port.TaskRepository, cache port.CacheRepository) (TaskCommandService, TaskQueryService, TaskCacheService) {
	s := &taskService{
		tasks:  tasks,
		cache:  cache,
		logger: logging2.GetLogger().GetLoggerWithField("service", "tasks"),
	}
	return s, s, s
}

// --- TaskCommandService ---

func (s *taskService) CreateTask(ctx context.Context, task domain.Task) error {
	if err := s.tasks.Create(ctx, task); err != nil {
		s.logger.Errorf("CreateTask: task %s user %s: %v", task.ID, task.UserID, err)
		return err
	}
	s.logger.Infof("CreateTask: task %s создана для user %s", task.ID, task.UserID)
	return nil
}

func (s *taskService) CreateTaskBatch(ctx context.Context, tasks []domain.Task) error {
	if err := s.tasks.CreateBatch(ctx, tasks); err != nil {
		s.logger.Errorf("CreateTaskBatch: %v", err)
		return err
	}
	s.logger.Infof("CreateTaskBatch: вставлено %d задач", len(tasks))
	return nil
}

// UpdateTask оркестрирует обновление: валидация перехода статуса → DB → лог.
func (s *taskService) UpdateTask(ctx context.Context, id string, patch port.UpdatePatch) (domain.Task, error) {
	if patch.Status != nil {
		current, err := s.tasks.FindByID(ctx, id)
		if err != nil {
			return domain.Task{}, fmt.Errorf("fetch task for update: %w", err)
		}
		if !current.CanTransitionTo(*patch.Status) {
			return domain.Task{}, fmt.Errorf("недопустимый переход статуса: %s → %s", current.Status, *patch.Status)
		}
	}
	updated, err := s.tasks.Update(ctx, id, patch)
	if err != nil {
		return domain.Task{}, fmt.Errorf("update task %s: %w", id, err)
	}
	s.logger.Infof("task %s обновлена", id)
	return updated, nil
}

func (s *taskService) DeleteTask(ctx context.Context, id string) error {
	if err := s.tasks.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete task %s: %w", id, err)
	}
	s.logger.Infof("task %s удалена", id)
	return nil
}

func (s *taskService) DeleteTaskBatch(ctx context.Context, ids []string) error {
	if err := s.tasks.DeleteBatch(ctx, ids); err != nil {
		s.logger.Errorf("DeleteTaskBatch: %v", err)
		return err
	}
	s.logger.Infof("DeleteTaskBatch: удалено %d задач", len(ids))
	return nil
}

// --- TaskQueryService ---

// FindTask — cache-first получение задачи.
func (s *taskService) FindTask(ctx context.Context, id string) (domain.Task, error) {
	cached, err := s.cache.GetTask(ctx, id)
	if err == nil && cached.ID != "" {
		return cached, nil
	}
	task, err := s.tasks.FindByID(ctx, id)
	if err != nil {
		s.logger.Errorf("FindTask: task %s: %v", id, err)
		return domain.Task{}, err
	}
	return task, nil
}

func (s *taskService) FindTasksByUser(ctx context.Context, userID string, sortOpts sort.Options, filterOpts filter.Option) ([]domain.Task, error) {
	tasks, err := s.tasks.FindByUser(ctx, userID, sortOpts, filterOpts)
	if err != nil {
		s.logger.Errorf("FindTasksByUser: user %s: %v", userID, err)
		return nil, err
	}
	return tasks, nil
}

func (s *taskService) FindTasksByTag(ctx context.Context, userID, tagID string) ([]domain.Task, error) {
	tasks, err := s.tasks.FindByTag(ctx, userID, tagID)
	if err != nil {
		s.logger.Errorf("FindTasksByTag: user %s tag %s: %v", userID, tagID, err)
		return nil, err
	}
	return tasks, nil
}

// --- TaskCacheService ---

func (s *taskService) SetTask(ctx context.Context, task domain.Task) error {
	return s.cache.SetTask(ctx, task)
}

func (s *taskService) GetTask(ctx context.Context, id string) (domain.Task, error) {
	return s.cache.GetTask(ctx, id)
}

func (s *taskService) DeleteCachedTask(ctx context.Context, id string) error {
	return s.cache.DeleteTask(ctx, id)
}

func (s *taskService) GetList(ctx context.Context, key string) ([]domain.Task, error) {
	return s.cache.GetList(ctx, key)
}

func (s *taskService) SetList(ctx context.Context, key string, tasks []domain.Task) error {
	return s.cache.SetList(ctx, key, tasks)
}

func (s *taskService) InvalidateUserLists(ctx context.Context, userID string) error {
	return s.cache.InvalidateUserLists(ctx, userID)
}
