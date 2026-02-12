package service

import (
	storage2 "TODOLIST_Tasks/app/internal/tags/storage"
	"TODOLIST_Tasks/app/internal/tasks/model"
	model2 "TODOLIST_Tasks/app/internal/tasks/storage/model"
	"TODOLIST_Tasks/app/internal/tasks/storage/postgres"
	"TODOLIST_Tasks/app/internal/tasks/storage/redis"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/api/sort"
	logging2 "TODOLIST_Tasks/app/pkg/logging"
	"context"
	"fmt"
	"time"
)

type Service struct {
	RepositoryTasks postgres.Repository
	RepositoryTags  storage2.Repository
	RepositoryRedis redis.Repository
	Logger          *logging2.Logger
}

func NewService(repositoryTasks postgres.Repository, repositoryTags storage2.Repository, repositoryRedis redis.Repository) *Service {
	return &Service{RepositoryTasks: repositoryTasks, RepositoryTags: repositoryTags, RepositoryRedis: repositoryRedis, Logger: logging2.GetLogger().GetLoggerWithField("service", "tasks")}
}

func (s *Service) FindOneRedis(ctx context.Context, id string) (model.Task, error) {
	task, err := s.RepositoryRedis.GetTaskFromCache(ctx, id)
	if err != nil {
		return model.Task{}, err
	}
	return task, nil
}

func (s *Service) DeleteTaskRedis(ctx context.Context, id string) error {
	err := s.RepositoryRedis.DeleteTaskCache(ctx, id)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) UpdateTaskRedis(ctx context.Context, task model.Task) error {

	err := s.RepositoryRedis.CacheTask(ctx, task)
	if err != nil {
		return err
	}
	return nil
}

// CreateTask - обратная совместимость
func (s *Service) CreateTask(ctx context.Context, task model.Task) error {
	err := s.RepositoryTasks.CreateTask(ctx, task)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) CreateTaskRedis(ctx context.Context, task model.Task) error {
	s.Logger.Info("🔴 Service.CreateTaskRedis START")
	start := time.Now()

	err := s.RepositoryRedis.CacheTask(ctx, task)

	if err != nil {
		s.Logger.Errorf("❌ Redis cache error: %v", err)
	} else {
		s.Logger.Infof("✅ Redis cache took: %v", time.Since(start))
	}

	return err
}

func (s *Service) UpdateTask(ctx context.Context, id string, task model.TaskUpdateDTO) (model.Task, error) {
	// Обновление в Postgres (теперь в репозитории автоматически сохраняется в outbox)
	result, err := s.RepositoryTasks.UpdateTask(ctx, id, task)
	if err != nil {
		return model.Task{}, fmt.Errorf("ошибка обновления задачи: %w", err)
	}

	s.Logger.Infof("Task %s updated successfully", id)
	return result, nil
}

func (s *Service) DeleteTask(ctx context.Context, id string) (string, error) {
	// Удаление в Postgres (теперь в репозитории автоматически сохраняется в outbox)
	result, err := s.RepositoryTasks.DeleteTask(ctx, id)
	if err != nil {
		return "", fmt.Errorf("ошибка удаления задачи: %w", err)
	}

	s.Logger.Infof("Task %s deleted successfully", id)
	return result, nil
}

func (s *Service) FindOneTask(ctx context.Context, id string) (model.Task, error) {

	task, err := s.RepositoryTasks.FindOneTask(ctx, id)
	if err != nil {
		return model.Task{}, err
	}
	return task, nil
}

func (s *Service) FindAllByTag(ctx context.Context, userId string, tagId string) ([]model.Task, error) {
	tasks, err := s.RepositoryTasks.FindAllByTag(ctx, userId, tagId)
	if err != nil {
		return []model.Task{}, err
	}
	return tasks, nil
}

func (s *Service) FindAllTasks(ctx context.Context, sortOptions sort.Options, filterOptions filter.Option, userId string) ([]model.Task, error) {
	// Проверяем существует ли пользователь

	fOptions := model2.NewFilterOptions(filterOptions)

	sOptions := model2.NewSortOptions(sortOptions.Fields, sortOptions.Order)

	// Теперь используем переданный filterOptions
	tasks, err := s.RepositoryTasks.FindAllTasks(ctx, sOptions, fOptions, userId)
	if err != nil {
		return []model.Task{}, err
	}
	return tasks, nil
}

func (s *Service) GetTasksFromCache(ctx context.Context, cacheKey string) ([]model.Task, error) {
	tasks, err := s.RepositoryRedis.GetTasksFromCacheList(ctx, cacheKey)
	if err != nil {
		return nil, fmt.Errorf("ошибка при получении задач из кэша: %w", err)
	}
	return tasks, nil
}

func (s *Service) SetTasksToCache(ctx context.Context, cacheKey string, tasks []model.Task) error {
	err := s.RepositoryRedis.SetTasksToCacheList(ctx, cacheKey, tasks)
	if err != nil {
		return fmt.Errorf("ошибка при записи задач в кэш: %w", err)
	}
	return nil
}
