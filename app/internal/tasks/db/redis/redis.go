package redis

import (
	model2 "TODOLIST_Tasks/app/internal/tasks/model"
	redis2 "TODOLIST_Tasks/app/internal/tasks/storage/redis"
	"TODOLIST_Tasks/app/pkg/client/redis"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	redis3 "github.com/go-redis/redis/v8"
	"time"
)

type repositoryRedis struct {
	Client redis.Client
}

func (r *repositoryRedis) CacheTask(ctx context.Context, task model2.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	// Ключ для хранения самой задачи
	key := fmt.Sprintf("task:%s", task.Id)
	err = r.Client.Set(ctx, key, data, 24*time.Hour).Err()
	if err != nil {
		return err
	}

	// Ключи для множества задач по userID и tagID
	userKey := fmt.Sprintf("user_tasks:%s", task.UserID)
	tagKey := fmt.Sprintf("tag_tasks:%s", task.TagID)

	// Добавляем идентификатор задачи в множества
	r.Client.SAdd(ctx, userKey, task.Id)
	r.Client.SAdd(ctx, tagKey, task.Id)

	return nil
}

// Удаление задачи из кэша

func (r *repositoryRedis) DeleteTaskCache(ctx context.Context, id string) error {
	key := fmt.Sprintf("task:%s", id)
	err := r.Client.Del(ctx, key).Err()
	if err != nil {
		return err
	}

	return nil
}

func (r *repositoryRedis) GetTaskFromCache(ctx context.Context, id string) (model2.Task, error) {
	key := fmt.Sprintf("task:%s", id)
	result, err := r.Client.Get(ctx, key).Result()
	if err != nil {
		return model2.Task{}, err
	}

	var task model2.Task
	if err := json.Unmarshal([]byte(result), &task); err != nil {
		return model2.Task{}, err
	}

	return task, nil
}

func (r *repositoryRedis) GetTasksFromCacheList(ctx context.Context, cacheKey string) ([]model2.Task, error) {
	cachedData, err := r.Client.Get(ctx, cacheKey).Result()
	if err != nil {
		if errors.Is(err, redis3.Nil) {
			// Если данных нет в кэше, возвращаем пустой срез
			return nil, nil
		}
		return nil, err
	}

	var tasks []model2.Task
	if err := json.Unmarshal([]byte(cachedData), &tasks); err != nil {
		return nil, fmt.Errorf("не удалось десериализовать данные из кэша: %w", err)
	}
	return tasks, nil
}

// Кэшируем задачи
func (r *repositoryRedis) SetTasksToCacheList(ctx context.Context, cacheKey string, tasks []model2.Task) error {
	data, err := json.Marshal(tasks)
	if err != nil {
		return err
	}

	// Сохраняем данные в кэш с TTL
	err = r.Client.Set(ctx, cacheKey, data, 24*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("не удалось сохранить задачи в кэш: %w", err)
	}
	return nil
}

func (r *repositoryRedis) GetTasksByTagID(ctx context.Context, tagId string) ([]model2.Task, error) {
	tagKey := fmt.Sprintf("tag_tasks:%s", tagId)
	taskIds, err := r.Client.SMembers(ctx, tagKey).Result()
	if err != nil {
		return nil, err
	}

	var tasks []model2.Task
	for _, taskId := range taskIds {
		taskData, err := r.Client.Get(ctx, fmt.Sprintf("task:%s", taskId)).Result()
		if err != nil {
			return nil, err
		}

		var task model2.Task
		if err := json.Unmarshal([]byte(taskData), &task); err != nil {
			return nil, err
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

func NewRepositoryRedis(client redis.Client) redis2.Repository {
	return &repositoryRedis{Client: client}
}
