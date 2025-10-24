package storage

import (
	"TODOLIST_Tasks/app/internal/tags/model"
	"context"
)

type Repository interface {
	FindTagByID(ctx context.Context, id string) (bool, error)
	CreateTags(ctx context.Context, tags model.Tags, userID string) (string, error)
	UpdateTags(ctx context.Context, id string, task model.TagsDTO, userID string) error
	DeleteTags(ctx context.Context, id string, userID string) error
	FindOneTags(ctx context.Context, id string, userID string) (model.Tags, error)
	FindAllByUser(ctx context.Context, userId string) ([]model.Tags, error)
}
