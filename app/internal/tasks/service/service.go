package service

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/model"
	"TODOLIST_Tasks/app/internal/tasks/port"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/api/sort"
	logging2 "TODOLIST_Tasks/app/pkg/logging"
	"context"
	"fmt"
)

type taskService struct {
	tasks    port.TaskRepository
	subtasks port.SubtaskRepository
	cache    port.CacheRepository
	logger   *logging2.Logger
}

// New создаёт сервис и возвращает пять интерфейсов через один объект.
func New(tasks port.TaskRepository, subtasks port.SubtaskRepository, cache port.CacheRepository) (TaskCommandService, TaskQueryService, TaskCacheService, SubtaskService, TaskAdminService) {
	s := &taskService{
		tasks:    tasks,
		subtasks: subtasks,
		cache:    cache,
		logger:   logging2.GetLogger().GetLoggerWithField("service", "tasks"),
	}
	return s, s, s, s, s
}

// --- TaskCommandService ---

func (s *taskService) CountActiveTasks(ctx context.Context, userID string) (int, error) {
	return s.tasks.CountActive(ctx, userID)
}

func (s *taskService) CreateTask(ctx context.Context, task domain.Task) error {
	if err := s.tasks.Create(ctx, task); err != nil {
		s.logger.Errorf("CreateTask: task %s user %s: %v", task.ID, task.UserID, err)
		return err
	}
	if task.HasSubtasks() {
		if err := s.subtasks.CreateSubtasks(ctx, task.ID, task.Subtasks); err != nil {
			s.logger.Errorf("CreateTask: subtasks for task %s: %v", task.ID, err)
			return err
		}
	}
	s.logger.Infof("CreateTask: task %s создана для user %s", task.ID, task.UserID)
	return nil
}

// CreateTaskBatch не поддерживает задачи с подзадачами — они должны идти через CreateTask.
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
// Подзадачи всегда загружаются из БД: они могут меняться независимо (toggle/rename/delete),
// а старый кэш мог быть записан без поля subtasks.
// Возвращает задачу и флаг fromCache: true — основные данные из Redis, false — из БД.
func (s *taskService) FindTask(ctx context.Context, id string) (domain.Task, bool, error) {
	cached, err := s.cache.GetTask(ctx, id)
	if err == nil && cached.ID != "" {
		// Всегда подгружаем актуальные подзадачи из БД
		subtasks, subErr := s.subtasks.FindByTask(ctx, id)
		if subErr == nil {
			cached.Subtasks = subtasks
		}
		return cached, true, nil
	}
	task, err := s.tasks.FindByID(ctx, id)
	if err != nil {
		s.logger.Errorf("FindTask: task %s: %v", id, err)
		return domain.Task{}, false, err
	}
	return task, false, nil
}

func (s *taskService) FindTasksByUser(ctx context.Context, userID string, sortOpts sort.Options, filterOpts filter.Option) ([]model.TaskList, error) {
	tasks, err := s.tasks.FindByUser(ctx, userID, sortOpts, filterOpts)
	if err != nil {
		s.logger.Errorf("FindTasksByUser: user %s: %v", userID, err)
		return nil, err
	}
	return tasks, nil
}

func (s *taskService) FindTasksByTag(ctx context.Context, userID, tagID string) ([]model.TaskList, error) {
	tasks, err := s.tasks.FindByTag(ctx, userID, tagID)
	if err != nil {
		s.logger.Errorf("FindTasksByTag: user %s tag %s: %v", userID, tagID, err)
		return nil, err
	}
	return tasks, nil
}

func (s *taskService) AdminFindAllTasks(ctx context.Context) ([]model.TaskList, error) {
	tasks, err := s.tasks.FindAll(ctx)
	if err != nil {
		s.logger.Errorf("AdminFindAllTasks: %v", err)
		return nil, err
	}
	return tasks, nil
}

func (s *taskService) AdminFindAllFiltered(ctx context.Context, from, to, status, priory string) ([]model.TaskList, error) {
	tasks, err := s.tasks.FindAllFiltered(ctx, from, to, status, priory)
	if err != nil {
		s.logger.Errorf("AdminFindAllFiltered: %v", err)
		return nil, err
	}
	return tasks, nil
}

func (s *taskService) AdminSoftDelete(ctx context.Context, id string) (string, error) {
	return s.tasks.AdminSoftDelete(ctx, id)
}

func (s *taskService) AcknowledgeAdminDeletion(ctx context.Context, id, userID string) error {
	return s.tasks.AcknowledgeAdminDeletion(ctx, id, userID)
}

func (s *taskService) AdminSoftDeleteShared(ctx context.Context, id string) (string, string, string, error) {
	return s.tasks.AdminSoftDeleteShared(ctx, id)
}

func (s *taskService) AcknowledgeAdminDeletionShared(ctx context.Context, id, userID string) error {
	return s.tasks.AcknowledgeAdminDeletionShared(ctx, id, userID)
}

func (s *taskService) AdminRestore(ctx context.Context, id string) (string, error) {
	return s.tasks.AdminRestore(ctx, id)
}

func (s *taskService) AdminRestoreShared(ctx context.Context, id string) (string, string, error) {
	return s.tasks.AdminRestoreShared(ctx, id)
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

func (s *taskService) GetList(ctx context.Context, key string) ([]model.TaskList, error) {
	return s.cache.GetList(ctx, key)
}

func (s *taskService) SetList(ctx context.Context, key string, tasks []model.TaskList) error {
	return s.cache.SetList(ctx, key, tasks)
}

func (s *taskService) InvalidateUserLists(ctx context.Context, userID string) error {
	return s.cache.InvalidateUserLists(ctx, userID)
}

// --- SubtaskService ---

func (s *taskService) AddSubtask(ctx context.Context, taskID, ownerID, title string) (domain.Subtask, error) {
	subtask, err := s.subtasks.CreateSubtask(ctx, taskID, ownerID, title)
	if err != nil {
		s.logger.Errorf("AddSubtask: task %s owner %s: %v", taskID, ownerID, err)
		return domain.Subtask{}, err
	}
	return subtask, nil
}

// ToggleSubtaskDone переключает выполненность подзадачи.
// Бизнес-правило: если после toggle все подзадачи задачи выполнены → задача автоматически получает статус "completed".
func (s *taskService) ToggleSubtaskDone(ctx context.Context, subtaskID, taskOwnerID string) (domain.Subtask, error) {
	subtask, err := s.subtasks.ToggleDone(ctx, subtaskID, taskOwnerID)
	if err != nil {
		s.logger.Errorf("ToggleSubtaskDone: subtask %s owner %s: %v", subtaskID, taskOwnerID, err)
		return domain.Subtask{}, err
	}

	// Авто-complete: если подзадача только что выполнена → проверяем все остальные
	if subtask.IsDone {
		allDone, err := s.subtasks.AreAllDone(ctx, subtask.TaskID)
		if err != nil {
			s.logger.Warnf("ToggleSubtaskDone: AreAllDone task %s: %v", subtask.TaskID, err)
			// Не блокируем ответ — подзадача уже переключена
			return subtask, nil
		}
		if allDone {
			status := domain.StatusCompleted
			if _, err := s.tasks.Update(ctx, subtask.TaskID, port.UpdatePatch{Status: &status}); err != nil {
				s.logger.Warnf("ToggleSubtaskDone: auto-complete task %s: %v", subtask.TaskID, err)
			} else {
				s.logger.Infof("task %s авто-завершена: все подзадачи выполнены", subtask.TaskID)
			}
		}
	}

	return subtask, nil
}

func (s *taskService) DeleteSubtask(ctx context.Context, subtaskID, taskOwnerID string) error {
	if err := s.subtasks.DeleteSubtask(ctx, subtaskID, taskOwnerID); err != nil {
		s.logger.Errorf("DeleteSubtask: subtask %s owner %s: %v", subtaskID, taskOwnerID, err)
		return err
	}
	return nil
}

func (s *taskService) UpdateSubtask(ctx context.Context, subtaskID, taskOwnerID, title string) error {
	if err := s.subtasks.UpdateSubtask(ctx, subtaskID, taskOwnerID, title); err != nil {
		s.logger.Errorf("UpdateSubtask: subtask %s owner %s: %v", subtaskID, taskOwnerID, err)
		return err
	}
	return nil
}
