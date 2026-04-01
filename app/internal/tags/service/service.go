package service

import (
	model3 "TODOLIST_Tasks/app/internal/tags/model"
	storage2 "TODOLIST_Tasks/app/internal/tags/storage"
	logging2 "TODOLIST_Tasks/app/pkg/logging"
	"context"
	"errors"
	"fmt"
)

type Service struct {
	repositoryPostgres storage2.Repository
	repositoryRedis    storage2.RepositoryRedis
	logger             *logging2.Logger
}

func NewService(postgres storage2.Repository, redis storage2.RepositoryRedis) *Service {
	return &Service{
		repositoryPostgres: postgres,
		repositoryRedis:    redis,
		logger:             logging2.GetLogger().GetLoggerWithField("service", "tags"),
	}
}

func (s *Service) CreateTags(ctx context.Context, tags model3.Tags, userID string) (string, error) {
	id, err := s.repositoryPostgres.CreateTags(ctx, tags, userID)
	if err != nil {
		s.logger.Errorf("CreateTags: user %s, tag '%s': %v", userID, tags.Name, err)
		return "", err
	}
	s.logger.Infof("CreateTags: created tag %s for user %s", id, userID)
	return id, nil
}

func (s *Service) CreateTagsRedis(ctx context.Context, tags model3.Tags, userID string) error {
	return s.repositoryRedis.CreateTagsRedis(ctx, tags, userID)
}

func (s *Service) UpdateTagsRedis(ctx context.Context, id string, tags model3.TagsDTO, userID string) error {
	return s.repositoryRedis.UpdateTagsRedis(ctx, id, tags, userID)
}

func (s *Service) DeleteTagsRedis(ctx context.Context, id string, userID string) error {
	return s.repositoryRedis.DeleteTagsRedis(ctx, id, userID)
}

func (s *Service) FindOneTagsRedis(ctx context.Context, id string, userID string) (model3.Tags, error) {
	return s.repositoryRedis.FindOneTagsRedis(ctx, id, userID)
}

func (s *Service) FindAllTagsRedis(ctx context.Context, userId string) ([]model3.Tags, error) {
	return s.repositoryRedis.FindAllTagsRedis(ctx, userId)
}

func (s *Service) SetTagList(ctx context.Context, cacheKey string, tags []model3.Tags) error {
	return s.repositoryRedis.SetTagToCacheList(ctx, cacheKey, tags)
}

// InvalidateTagListCache сбрасывает кэш списка тегов пользователя.
func (s *Service) InvalidateTagListCache(ctx context.Context, userID string) error {
	return s.repositoryRedis.DeleteTagListCache(ctx, userID)
}

func (s *Service) UpdateTags(ctx context.Context, id string, tags model3.TagsDTO, userID string) error {
	ok, _ := s.repositoryPostgres.FindTagByID(ctx, id)
	if !ok {
		s.logger.Warnf("UpdateTags: tag %s not found for user %s", id, userID)
		return errors.New("tag not found")
	}

	if err := s.repositoryPostgres.UpdateTags(ctx, id, tags, userID); err != nil {
		s.logger.Errorf("UpdateTags: tag %s user %s: %v", id, userID, err)
		return fmt.Errorf("failed to update tag: %w", err)
	}

	s.logger.Infof("UpdateTags: tag %s updated for user %s", id, userID)
	return nil
}

func (s *Service) DeleteTags(ctx context.Context, id string, userID string) error {
	ok, _ := s.repositoryPostgres.FindTagByID(ctx, id)
	if !ok {
		s.logger.Warnf("DeleteTags: tag %s not found for user %s", id, userID)
		return errors.New("tag not found")
	}
	if err := s.repositoryPostgres.DeleteTags(ctx, id, userID); err != nil {
		s.logger.Errorf("DeleteTags: tag %s user %s: %v", id, userID, err)
		return err
	}
	s.logger.Infof("DeleteTags: tag %s deleted for user %s", id, userID)
	return nil
}

func (s *Service) FindOneTags(ctx context.Context, id string, userID string) (model3.Tags, error) {
	tag, err := s.repositoryPostgres.FindOneTags(ctx, id, userID)
	if err != nil {
		s.logger.Errorf("FindOneTags: tag %s user %s: %v", id, userID, err)
		return model3.Tags{}, err
	}
	return tag, nil
}

func (s *Service) FindAllTags(ctx context.Context, userId string) ([]model3.Tags, error) {
	tags, err := s.repositoryPostgres.FindAllByUser(ctx, userId)
	if err != nil {
		s.logger.Errorf("FindAllTags: user %s: %v", userId, err)
		return nil, err
	}
	return tags, nil
}
