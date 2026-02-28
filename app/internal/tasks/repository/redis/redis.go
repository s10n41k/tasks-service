package redis

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/port"
	redisclient "TODOLIST_Tasks/app/pkg/client/redis"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	redis3 "github.com/go-redis/redis/v8"
)

type repo struct {
	client redisclient.Client
}

func NewRepository(client redisclient.Client) port.CacheRepository {
	return &repo{client: client}
}

// cacheTask — внутренняя структура для JSON-сериализации в Redis.
// Использует те же json-ключи что и старый model.Task для обратной совместимости.
type cacheTask struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Priority    string    `json:"priory"`
	Status      string    `json:"status"`
	DueDate     time.Time `json:"due_date"`
	UserID      string    `json:"user_id"`
	TagID       *string   `json:"tag_id,omitempty"`
	TagName     string    `json:"tags_name"`
	CreatedAt   time.Time `json:"created_at"`
}

func toCacheTask(t domain.Task) cacheTask {
	return cacheTask{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Priority:    t.Priority,
		Status:      string(t.Status),
		DueDate:     t.DueDate,
		UserID:      t.UserID,
		TagID:       t.TagID,
		TagName:     t.TagName,
		CreatedAt:   t.CreatedAt,
	}
}

func fromCacheTask(c cacheTask) domain.Task {
	return domain.Task{
		ID:          c.ID,
		Title:       c.Title,
		Description: c.Description,
		Priority:    c.Priority,
		Status:      domain.NewStatus(c.Status),
		DueDate:     c.DueDate,
		UserID:      c.UserID,
		TagID:       c.TagID,
		TagName:     c.TagName,
		CreatedAt:   c.CreatedAt,
	}
}

func (r *repo) SetTask(ctx context.Context, task domain.Task) error {
	data, err := json.Marshal(toCacheTask(task))
	if err != nil {
		return err
	}

	key := fmt.Sprintf("task:%s", task.ID)
	userKey := fmt.Sprintf("user_tasks:%s", task.UserID)

	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, data, 24*time.Hour)
	pipe.SAdd(ctx, userKey, task.ID)
	if task.TagID != nil && *task.TagID != "" {
		pipe.SAdd(ctx, fmt.Sprintf("tag_tasks:%s", *task.TagID), task.ID)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (r *repo) GetTask(ctx context.Context, id string) (domain.Task, error) {
	result, err := r.client.Get(ctx, fmt.Sprintf("task:%s", id)).Result()
	if err != nil {
		return domain.Task{}, err
	}
	var c cacheTask
	if err := json.Unmarshal([]byte(result), &c); err != nil {
		return domain.Task{}, err
	}
	return fromCacheTask(c), nil
}

func (r *repo) DeleteTask(ctx context.Context, id string) error {
	return r.client.Del(ctx, fmt.Sprintf("task:%s", id)).Err()
}

func (r *repo) GetList(ctx context.Context, key string) ([]domain.Task, error) {
	data, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis3.Nil) {
			return nil, nil
		}
		return nil, err
	}
	var cached []cacheTask
	if err := json.Unmarshal([]byte(data), &cached); err != nil {
		return nil, fmt.Errorf("deserialize cached tasks: %w", err)
	}
	tasks := make([]domain.Task, len(cached))
	for i, c := range cached {
		tasks[i] = fromCacheTask(c)
	}
	return tasks, nil
}

func (r *repo) SetList(ctx context.Context, key string, tasks []domain.Task) error {
	cached := make([]cacheTask, len(tasks))
	for i, t := range tasks {
		cached[i] = toCacheTask(t)
	}
	data, err := json.Marshal(cached)
	if err != nil {
		return err
	}

	userListSetKey := userListKey(key)
	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, data, 24*time.Hour)
	if userListSetKey != "" {
		pipe.SAdd(ctx, userListSetKey, key)
		pipe.Expire(ctx, userListSetKey, 25*time.Hour)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (r *repo) InvalidateUserLists(ctx context.Context, userID string) error {
	userListSetKey := fmt.Sprintf("user_list_keys:%s", userID)

	keys, err := r.client.SMembers(ctx, userListSetKey).Result()
	if err != nil {
		if errors.Is(err, redis3.Nil) {
			return nil
		}
		return fmt.Errorf("get user list keys: %w", err)
	}
	if len(keys) == 0 {
		return nil
	}
	return r.client.Del(ctx, append(keys, userListSetKey)...).Err()
}

// userListKey извлекает ключ tracking-сета из cache key формата "tasks:user:<id>[;...]".
func userListKey(cacheKey string) string {
	base := strings.SplitN(cacheKey, ";", 2)[0]
	parts := strings.Split(base, ":")
	if len(parts) >= 3 && parts[0] == "tasks" && parts[1] == "user" {
		return fmt.Sprintf("user_list_keys:%s", parts[2])
	}
	return ""
}
