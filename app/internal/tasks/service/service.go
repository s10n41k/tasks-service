package service

import (
	storage2 "TODOLIST_Tasks/app/internal/tags/storage"
	"TODOLIST_Tasks/app/internal/tasks/event"
	"TODOLIST_Tasks/app/internal/tasks/model"
	model2 "TODOLIST_Tasks/app/internal/tasks/storage/model"
	"TODOLIST_Tasks/app/internal/tasks/storage/postgres"
	"TODOLIST_Tasks/app/internal/tasks/storage/redis"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/api/sort"
	logging2 "TODOLIST_Tasks/app/pkg/logging"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Service struct {
	RepositoryTasks postgres.Repository
	RepositoryTags  storage2.Repository
	RepositoryRedis redis.Repository
	RepositoryKafka event.Repository
	Logger          *logging2.Logger
}

func NewService(repositoryTasks postgres.Repository, repositoryTags storage2.Repository, repositoryRedis redis.Repository, repositoryKafka event.Repository) *Service {
	return &Service{RepositoryTasks: repositoryTasks, RepositoryTags: repositoryTags, RepositoryRedis: repositoryRedis, RepositoryKafka: repositoryKafka, Logger: logging2.GetLogger().GetLoggerWithField("service", "tasks")}
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

func (s *Service) CreateTaskRedis(ctx context.Context, task model.Task) error {
	err := s.RepositoryRedis.CacheTask(ctx, task)
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

func (s *Service) CreateTask(ctx context.Context, task model.Task) (model.Task, error) {

	if task.TagID != "" {
		tag, err := s.RepositoryTags.FindOneTags(ctx, task.TagID, task.UserID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return model.Task{}, fmt.Errorf("тег с ID %s не найден", task.TagID)
			}
			return model.Task{}, fmt.Errorf("ошибка при поиске тега по ID: %w", err)
		}
		task.TagsName = tag.Name
	}

	// Создаем задачу
	taskID, err := s.RepositoryTasks.CreateTask(ctx, task)
	if err != nil {
		return model.Task{}, fmt.Errorf("ошибка при создании задачи: %w", err)
	}

	taskResult, err := s.RepositoryTasks.FindOneTask(ctx, taskID)
	if err != nil {
		return model.Task{}, fmt.Errorf("ошибка при поиске задачи: %w", err)
	}
	go func(task model.Task) {
		for i := 0; i < 3; i++ {
			if err = s.RepositoryKafka.TaskCreated(context.Background(), task); err == nil {
				s.Logger.Errorf("Ошибка при записи в Кафку: %v", err)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}(taskResult)

	return taskResult, nil
}

func (s *Service) UpdateTask(ctx context.Context, id string, task model.TaskUpdateDTO) (model.Task, error) {

	result, err := s.RepositoryTasks.UpdateTask(ctx, id, task)
	if err != nil {
		return model.Task{}, err
	}
	return result, nil
}

func (s *Service) DeleteTask(ctx context.Context, id string) (string, error) {

	result, err := s.RepositoryTasks.DeleteTask(ctx, id)
	if err != nil {
		return "Couldn't delete a task", err
	}
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
