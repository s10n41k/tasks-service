package storage

import (
	"TODOLIST_Tasks/app/internal/tags/model"
	"context"
)

type RepositoryRedis interface {
	CreateTagsRedis(ctx context.Context, tags model.Tags, userID string) error
	UpdateTagsRedis(ctx context.Context, id string, tags model.TagsDTO, userID string) error
	DeleteTagsRedis(ctx context.Context, id string, userID string) error
	FindOneTagsRedis(ctx context.Context, id string, userID string) (model.Tags, error)
	FindAllTagsRedis(ctx context.Context, userId string) ([]model.Tags, error)
	SetTagToCacheList(ctx context.Context, cacheKey string, tags []model.Tags) error
	// DeleteTagListCache инвалидирует кэш всего списка тегов пользователя
	DeleteTagListCache(ctx context.Context, userID string) error
}
