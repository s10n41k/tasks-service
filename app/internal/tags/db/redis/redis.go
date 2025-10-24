package redis

import (
	"TODOLIST_Tasks/app/internal/tags/model"
	"TODOLIST_Tasks/app/internal/tags/storage"
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

func (r *repositoryRedis) CreateTagsRedis(ctx context.Context, tags model.Tags, userID string) error {
	data, err := json.Marshal(tags)
	if err != nil {
		return err
	}

	// Ключ с userID и tag ID
	key := fmt.Sprintf("user:%s:tag:%s", userID, tags.Id)

	err = r.Client.Set(ctx, key, data, 24*time.Hour).Err()
	if err != nil {
		return err
	}

	return nil
}

func (r *repositoryRedis) UpdateTagsRedis(ctx context.Context, id string, tags model.TagsDTO, userID string) error {
	data, err := json.Marshal(tags)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("user:%s:tag:%s", id, userID)
	err = r.Client.Set(ctx, key, data, 24*time.Hour).Err()
	if err != nil {
		return err
	}

	return nil
}

func (r *repositoryRedis) DeleteTagsRedis(ctx context.Context, id string, userID string) error {
	key := fmt.Sprintf("user:%s:tag:%s", id, userID)
	err := r.Client.Del(ctx, key).Err()
	if err != nil {
		return err
	}

	return nil
}

func (r *repositoryRedis) FindOneTagsRedis(ctx context.Context, id string, userID string) (model.Tags, error) {
	key := fmt.Sprintf("user:%s:tag:%s", id, userID)
	result, err := r.Client.Get(ctx, key).Result()
	if err != nil {
		return model.Tags{}, err
	}

	var tag model.Tags
	if err = json.Unmarshal([]byte(result), &tag); err != nil {
		return model.Tags{}, err
	}

	return tag, nil
}

func (r *repositoryRedis) FindAllTagsRedis(ctx context.Context, userId string) ([]model.Tags, error) {
	cachedData, err := r.Client.Get(ctx, userId).Result()
	if err != nil {
		if errors.Is(err, redis3.Nil) {
			// Если данных нет в кэше, возвращаем пустой срез
			return nil, nil
		}
		return nil, err
	}

	var tag []model.Tags
	if err = json.Unmarshal([]byte(cachedData), &tag); err != nil {
		return nil, fmt.Errorf("не удалось десериализовать данные из кэша: %w", err)
	}
	return tag, nil
}

func (r *repositoryRedis) SetTagToCacheList(ctx context.Context, cacheKey string, tags []model.Tags) error {
	data, err := json.Marshal(tags)
	if err != nil {
		return err
	}

	// Сохраняем данные в кэш с TTL
	err = r.Client.Set(ctx, cacheKey, data, 24*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("не удалось сохранить теги в кэш: %w", err)
	}
	return nil
}

func NewRepositoryRedis(client redis.Client) storage.RepositoryRedis {
	return &repositoryRedis{Client: client}
}
