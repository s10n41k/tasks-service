package service

import (
	model3 "TODOLIST_Tasks/app/internal/tags/model"
	storage2 "TODOLIST_Tasks/app/internal/tags/storage"
	"context"
	"errors"
	"fmt"
)

type Service struct {
	repositoryPostgres storage2.Repository
	repositoryRedis    storage2.RepositoryRedis
}

func NewService(postgres storage2.Repository, redis storage2.RepositoryRedis) *Service {
	return &Service{repositoryPostgres: postgres, repositoryRedis: redis}
}

func (s *Service) CreateTags(ctx context.Context, tags model3.Tags, userID string) (string, error) {

	id, err := s.repositoryPostgres.CreateTags(ctx, tags, userID)
	if err != nil {
		return fmt.Sprintf("err: %v", err), err
	}
	return id, nil
}

func (s *Service) CreateTagsRedis(ctx context.Context, tags model3.Tags, userID string) error {
	err := s.repositoryRedis.CreateTagsRedis(ctx, tags, userID)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) UpdateTagsRedis(ctx context.Context, id string, tags model3.TagsDTO, userID string) error {
	err := s.repositoryRedis.UpdateTagsRedis(ctx, id, tags, userID)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) DeleteTagsRedis(ctx context.Context, id string, userID string) error {
	err := s.repositoryRedis.DeleteTagsRedis(ctx, id, userID)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) FindOneTagsRedis(ctx context.Context, id string, userID string) (model3.Tags, error) {
	result, err := s.repositoryRedis.FindOneTagsRedis(ctx, id, userID)
	if err != nil {
		return model3.Tags{}, err
	}
	return result, nil
}

func (s *Service) FindALlTagsRedis(ctx context.Context, userId string) ([]model3.Tags, error) {
	result, err := s.repositoryRedis.FindAllTagsRedis(ctx, userId)
	if err != nil {
		return []model3.Tags{}, err
	}
	return result, nil
}

func (s *Service) SetTagList(ctx context.Context, cacheKey string, tags []model3.Tags) error {
	if err := s.repositoryRedis.SetTagToCacheList(ctx, cacheKey, tags); err != nil {
		return err
	}
	return nil
}

func (s *Service) UpdateTags(ctx context.Context, id string, tags model3.TagsDTO, userID string) error {
	// Проверяем, существует ли тег с данным ID
	ok, err := s.repositoryPostgres.FindTagByID(ctx, id)
	if !ok {
		return errors.New("tag not found")
	}

	err = s.repositoryPostgres.UpdateTags(ctx, id, tags, userID)
	if err != nil {
		return fmt.Errorf("не удалось обновить тег: %w", err)
	}

	return nil
}

func (s *Service) DeleteTags(ctx context.Context, id string, userID string) error {
	ok, err := s.repositoryPostgres.FindTagByID(ctx, id)
	if !ok {
		return errors.New("tag not found")
	}
	err = s.repositoryPostgres.DeleteTags(ctx, id, userID)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) FindOneTags(ctx context.Context, id string, userID string) (model3.Tags, error) {
	//ok, err := s.repositoryPostgres.FindTagByID(ctx, id)
	//if !ok {
	//return model3.Tags{}, errors.New("tag not found")
	//}
	tags, err := s.repositoryPostgres.FindOneTags(ctx, id, userID)
	if err != nil {
		return model3.Tags{}, err
	}
	return tags, nil
}

func (s *Service) FindAllTags(ctx context.Context, userId string) ([]model3.Tags, error) {

	tags, err := s.repositoryPostgres.FindAllByUser(ctx, userId)
	if err != nil {
		return []model3.Tags{}, err
	}
	return tags, nil
}
