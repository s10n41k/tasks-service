package mock

import (
	"TODOLIST_Tasks/app/internal/tags/model"
	"context"
	"github.com/stretchr/testify/mock"
)

type TagsRepository struct {
	mock.Mock
}

func (m *TagsRepository) FindTagNameByID(ctx context.Context, tagID string) (string, error) {
	args := m.Called(ctx, tagID)
	return args.String(0), args.Error(1)
}

func (m *TagsRepository) FindTagByName(ctx context.Context, name string) (string, error) {
	args := m.Called(ctx, name)
	return args.String(0), args.Error(1)
}

func (m *TagsRepository) FindTagByID(ctx context.Context, id string) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *TagsRepository) CreateTags(ctx context.Context, tags model.Tags) (string, error) {
	args := m.Called(ctx, tags)
	return args.String(0), args.Error(1)
}

func (m *TagsRepository) UpdateTags(ctx context.Context, id string, tags model.TagsDTO) error {
	args := m.Called(ctx, id, tags)
	return args.Error(0)
}

func (m *TagsRepository) DeleteTags(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *TagsRepository) FindOneTags(ctx context.Context, id string) (model.Tags, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(model.Tags), args.Error(1)
}

func (m *TagsRepository) FindAllByUser(ctx context.Context, userId string) ([]model.Tags, error) {
	args := m.Called(ctx, userId)
	return args.Get(0).([]model.Tags), args.Error(1)
}
