package handlers

import (
	"TODOLIST_Tasks/app/internal/apperror"
	"TODOLIST_Tasks/app/internal/handlers"
	"TODOLIST_Tasks/app/internal/tags/model"
	service2 "TODOLIST_Tasks/app/internal/tags/service"
	"TODOLIST_Tasks/app/internal/tasks/port"
	logging2 "TODOLIST_Tasks/app/pkg/logging"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgconn"
	"github.com/julienschmidt/httprouter"
	"io"
	"net/http"
	"time"
)

const (
	tagURL        = "/v1/users/:userId/tags/:tagsId"
	tagsByUserURL = "/v1/users/:userId/tags"

	goroutineTimeout = 5 * time.Second
)

type handler struct {
	service   *service2.Service
	taskCache port.CacheRepository // для инвалидации кэша задач при изменении тегов
	logger    *logging2.Logger
}

func NewHandler(service *service2.Service, taskCache port.CacheRepository) handlers.Handler {
	return &handler{
		service:   service,
		taskCache: taskCache,
		logger:    logging2.GetLogger().GetLoggerWithField("handler", "tags"),
	}
}

func (h *handler) Register(router *httprouter.Router) {
	router.HandlerFunc(http.MethodPost, tagsByUserURL, apperror.Middleware(h.CreateTag))
	router.HandlerFunc(http.MethodPatch, tagURL, apperror.Middleware(h.UpdateTag))
	router.HandlerFunc(http.MethodGet, tagURL, apperror.Middleware(h.FindOne))
	router.HandlerFunc(http.MethodGet, tagsByUserURL, apperror.Middleware(h.GetList))
	router.HandlerFunc(http.MethodDelete, tagURL, apperror.Middleware(h.Delete))
}

func (h *handler) CreateTag(w http.ResponseWriter, r *http.Request) error {
	h.logger.Info("CreateTag called")

	params := httprouter.ParamsFromContext(r.Context())
	userId := params.ByName("userId")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Errorf("failed to read request body: %v", err)
		return err
	}
	defer r.Body.Close()

	var tagDTO model.TagsDTO
	if err := json.Unmarshal(body, &tagDTO); err != nil {
		h.logger.Errorf("failed to unmarshal request body: %v", err)
		return apperror.BadRequest("Invalid request payload", err.Error())
	}

	tag := model.Tags{
		Name:   tagDTO.Name,
		UserID: &userId,
	}

	ctx := r.Context()
	result, err := h.service.CreateTags(ctx, tag, userId)
	if err != nil {
		h.logger.Errorf("failed to create tag: %v", err)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return apperror.BadRequest(fmt.Sprintf("Тег «%s» уже существует", tag.Name), pgErr.Error())
		}
		return err
	}
	tag.Id = result

	h.logger.Infof("tag created successfully with id: %s", result)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": result, "name": tag.Name})

	go func(tag model.Tags, userId string) {
		bgCtx, bgCancel := context.WithTimeout(context.Background(), goroutineTimeout)
		defer bgCancel()
		if cacheErr := h.service.CreateTagsRedis(bgCtx, tag, userId); cacheErr != nil {
			h.logger.Errorf("failed to cache tag in Redis: %v", cacheErr)
		}
		// Инвалидируем кэш списка тегов, чтобы новый тег сразу появился в GetList
		if invErr := h.service.InvalidateTagListCache(bgCtx, userId); invErr != nil {
			h.logger.Errorf("failed to invalidate tag list cache after create: %v", invErr)
		}
	}(tag, userId)

	return nil
}

func (h *handler) Delete(w http.ResponseWriter, r *http.Request) error {
	h.logger.Info("DeleteTag called")

	params := httprouter.ParamsFromContext(r.Context())
	userId := params.ByName("userId")
	tagId := params.ByName("tagsId")
	if userId == "" || tagId == "" {
		h.logger.Warn("missing userId or tagId parameter in DeleteTag")
		http.Error(w, "Invalid user ID or tag ID", http.StatusBadRequest)
		return errors.New("invalid user ID or tag ID")
	}

	ctx := r.Context()
	err := h.service.DeleteTags(ctx, tagId, userId)
	if err != nil {
		h.logger.Errorf("failed to delete tag with ID %s: %v", tagId, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}

	h.logger.Infof("tag with ID %s deleted successfully", tagId)

	response := map[string]string{"message": "successful delete"}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	go func(tagId string, userId string) {
		bgCtx, bgCancel := context.WithTimeout(context.Background(), goroutineTimeout)
		defer bgCancel()
		if delErr := h.service.DeleteTagsRedis(bgCtx, tagId, userId); delErr != nil {
			h.logger.Errorf("failed to delete tag %s from Redis: %v", tagId, delErr)
		}
		// Инвалидируем кэш списка тегов
		if invErr := h.service.InvalidateTagListCache(bgCtx, userId); invErr != nil {
			h.logger.Errorf("failed to invalidate tag list cache after delete: %v", invErr)
		}
		// Инвалидируем кэши всех задач пользователя — иначе карточка задачи покажет удалённый тег
		if invErr := h.taskCache.InvalidateUserTaskCaches(bgCtx, userId); invErr != nil {
			h.logger.Errorf("failed to invalidate user task caches for tag delete %s: %v", tagId, invErr)
		}
	}(tagId, userId)

	return nil
}

func (h *handler) GetList(w http.ResponseWriter, r *http.Request) error {
	h.logger.Info("GetList called")

	params := httprouter.ParamsFromContext(r.Context())
	userId := params.ByName("userId")
	if userId == "" {
		h.logger.Warn("missing userId parameter in GetList")
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return nil
	}

	// Try Redis cache first
	tagsRedis, err := h.service.FindAllTagsRedis(r.Context(), userId)
	if err == nil && len(tagsRedis) > 0 {
		h.logger.Infof("found %d tags for userID %s in Redis", len(tagsRedis), userId)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(tagsRedis); err != nil {
			h.logger.Errorf("failed to encode Redis tags: %v", err)
			http.Error(w, "Error encoding response", http.StatusInternalServerError)
			return err
		}
		return nil
	}

	// Fallback to PostgreSQL
	tags, err := h.service.FindAllTags(r.Context(), userId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || len(tags) == 0 {
			h.logger.Warnf("no tags found for userID: %s", userId)
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
		h.logger.Errorf("error finding tags in PostgreSQL for userID %s: %v", userId, err)
		http.Error(w, "Error retrieving tags", http.StatusInternalServerError)
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(tags); err != nil {
		h.logger.Errorf("failed to encode response: %v", err)
		return err
	}

	h.logger.Infof("returned %d tags from DB for userID: %s", len(tags), userId)

	// Cache asynchronously
	go func(tags []model.Tags, userId string) {
		bgCtx, bgCancel := context.WithTimeout(context.Background(), goroutineTimeout)
		defer bgCancel()
		if cacheErr := h.service.SetTagList(bgCtx, userId, tags); cacheErr != nil {
			h.logger.Errorf("failed to cache tags in Redis: %v", cacheErr)
		}
	}(tags, userId)

	return nil
}

func (h *handler) FindOne(w http.ResponseWriter, r *http.Request) error {
	h.logger.Info("FindOneTag called")

	params := httprouter.ParamsFromContext(r.Context())
	userId := params.ByName("userId")
	tagId := params.ByName("tagsId")
	if userId == "" || tagId == "" {
		h.logger.Warn("missing userId or tagId parameter in FindOneTag")
		http.Error(w, "Invalid user ID or tag ID", http.StatusBadRequest)
		return errors.New("invalid user ID or tag ID")
	}

	// Try Redis cache first
	tagRedis, err := h.service.FindOneTagsRedis(r.Context(), tagId, userId)
	if err == nil && tagRedis.Id != "" {
		h.logger.Infof("tag with ID %s found in Redis", tagId)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(tagRedis); err != nil {
			h.logger.Errorf("failed to encode response: %v", err)
			http.Error(w, "Error encoding response", http.StatusInternalServerError)
			return err
		}
		return nil
	}

	// Fallback to PostgreSQL
	tags, err := h.service.FindOneTags(r.Context(), tagId, userId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.logger.Warnf("tag with ID %s not found", tagId)
			http.Error(w, "Tag not found", http.StatusNotFound)
			return nil
		}
		h.logger.Errorf("failed to retrieve tag with ID %s: %v", tagId, err)
		http.Error(w, "Failed to retrieve tag", http.StatusInternalServerError)
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(tags); err != nil {
		h.logger.Errorf("failed to encode response: %v", err)
		return err
	}

	h.logger.Infof("tag with ID %s retrieved successfully", tagId)

	// Cache asynchronously
	go func(tag model.Tags, userId string) {
		bgCtx, bgCancel := context.WithTimeout(context.Background(), goroutineTimeout)
		defer bgCancel()
		if cacheErr := h.service.CreateTagsRedis(bgCtx, tag, userId); cacheErr != nil {
			h.logger.Errorf("failed to cache tag in Redis: %v", cacheErr)
		}
	}(tags, userId)

	return nil
}

func (h *handler) UpdateTag(w http.ResponseWriter, r *http.Request) error {
	h.logger.Info("UpdateTag called")

	params := httprouter.ParamsFromContext(r.Context())
	userId := params.ByName("userId")
	tagId := params.ByName("tagsId")
	if userId == "" || tagId == "" {
		h.logger.Warn("missing userId or tagId parameter in UpdateTag")
		http.Error(w, "Invalid user ID or tag ID", http.StatusBadRequest)
		return errors.New("invalid user ID or tag ID")
	}

	var tagRequest model.TagsDTO
	if err := json.NewDecoder(r.Body).Decode(&tagRequest); err != nil {
		h.logger.Errorf("failed to decode request body: %v", err)
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return err
	}

	err := h.service.UpdateTags(r.Context(), tagId, tagRequest, userId)
	if err != nil {
		h.logger.Errorf("failed to update tag with ID %s: %v", tagId, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}

	h.logger.Infof("tag with ID %s updated successfully", tagId)

	response := map[string]string{"message": "successful update"}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	go func(tagId string, userId string, tagRequest model.TagsDTO) {
		bgCtx, bgCancel := context.WithTimeout(context.Background(), goroutineTimeout)
		defer bgCancel()
		// Обновляем кэш отдельного тега
		if delErr := h.service.DeleteTagsRedis(bgCtx, tagId, userId); delErr != nil {
			h.logger.Errorf("failed to delete tag from Redis: %v", delErr)
		}
		if updErr := h.service.UpdateTagsRedis(bgCtx, tagId, tagRequest, userId); updErr != nil {
			h.logger.Errorf("failed to update tag in Redis: %v", updErr)
		}
		// Инвалидируем кэш списка тегов — иначе GetList вернёт старое имя из кэша
		if invErr := h.service.InvalidateTagListCache(bgCtx, userId); invErr != nil {
			h.logger.Errorf("failed to invalidate tag list cache after update: %v", invErr)
		}
		// Инвалидируем кэши всех задач пользователя — иначе карточка задачи покажет старое имя тега
		if invErr := h.taskCache.InvalidateUserTaskCaches(bgCtx, userId); invErr != nil {
			h.logger.Errorf("failed to invalidate user task caches for tag update %s: %v", tagId, invErr)
		}
	}(tagId, userId, tagRequest)

	return nil
}
