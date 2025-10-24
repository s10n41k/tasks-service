package handlers

import (
	"TODOLIST_Tasks/app/internal/apperror"
	"TODOLIST_Tasks/app/internal/handlers"
	"TODOLIST_Tasks/app/internal/tags/model"
	service2 "TODOLIST_Tasks/app/internal/tags/service"
	logging2 "TODOLIST_Tasks/app/pkg/logging"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"io"
	"net/http"
)

const (
	tagURL        = "/v1/users/:userId/tags/:tagsId"
	tagsByUserURL = "/v1/users/:userId/tags"
)

type handler struct {
	service *service2.Service
	logger  *logging2.Logger // добавляем логгер
}

func NewHandler(service *service2.Service) handlers.Handler {
	return &handler{
		service: service,
		logger:  logging2.GetLogger().GetLoggerWithField("handler", "tags"),
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
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return err
	}
	defer r.Body.Close()

	var tagDTO model.TagsDTO
	if err = json.Unmarshal(body, &tagDTO); err != nil {
		h.logger.Errorf("failed to unmarshal request body: %v", err)
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return err
	}

	tag := model.Tags{
		Name:   tagDTO.Name,
		UserID: &userId,
	}

	ctx := r.Context()
	result, err := h.service.CreateTags(ctx, tag, userId)
	if err != nil {
		h.logger.Errorf("failed to create tag: %v", err)
		http.Error(w, "Failed to create tag", http.StatusInternalServerError)
		return err
	}
	tag.Id = result

	h.logger.Infof("Tag created successfully with id: %s", result)

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(fmt.Sprintf("Tag created: %s", result)))

	go func(result string, userId string) {
		if err = h.service.CreateTagsRedis(context.Background(), tag, userId); err != nil {
			h.logger.Errorf("failed to create tag redis: %v", err)
		}
	}(result, userId)

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

	h.logger.Infof("Tag with ID %s deleted successfully", tagId)

	response := map[string]string{"message": "successful delete"}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	go func(id string, userID string) {
		if err = h.service.DeleteTagsRedis(context.Background(), id, userId); err != nil {
			h.logger.Errorf("failed to delete tag redis with id %s: %v", id, err)
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

	// 1. Пробуем достать теги из Redis
	tagsRedis, err := h.service.FindALlTagsRedis(r.Context(), userId)
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

	// 2. Ищем в PostgreSQL, если в Redis нет
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

	// 3. Отправляем ответ клиенту
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(tags); err != nil {
		h.logger.Errorf("failed to encode response: %v", err)
		return err
	}

	h.logger.Infof("returned %d tags from DB for userID: %s", len(tags), userId)

	// 4. Кэшируем в Redis асинхронно
	go func(tags []model.Tags, userId string) {
		if err := h.service.SetTagList(context.Background(), userId, tags); err != nil {
			h.logger.Errorf("failed to cache tags in Redis: %v", err)
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

	tagRedis, err := h.service.FindOneTagsRedis(r.Context(), tagId, userId)
	if err == nil && tagRedis.Id != "" {
		h.logger.Infof("Task with ID %s and userID %s found in Redis", tagId, userId)

		// Отправляем задачу из Redis
		response := model.Tags{
			Id:     tagRedis.Id,
			Name:   tagRedis.Name,
			UserID: tagRedis.UserID,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			h.logger.Errorf("failed to encode response: %v", err)
			http.Error(w, "Error encoding response", http.StatusInternalServerError)
			return err
		}
		return nil
	}

	tags, err := h.service.FindOneTags(r.Context(), tagId, userId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.logger.Warnf("tag with ID %s and userID %s not found", tagId, userId)
			http.Error(w, "Tag not found", http.StatusNotFound)
			return nil
		}
		h.logger.Errorf("failed to retrieve tag with ID %s: %v", tagId, err)
		http.Error(w, "Failed to retrieve tag", http.StatusInternalServerError)
		return err
	}

	response := model.Tags{
		Id:     tags.Id,
		Name:   tags.Name,
		UserID: tags.UserID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		h.logger.Errorf("failed to encode response: %v", err)
		return err
	}

	h.logger.Infof("tag with ID %s retrieved successfully", tagId)

	go func(tag model.Tags, userId string) {
		if err = h.service.CreateTagsRedis(context.Background(), tags, userId); err != nil {
			h.logger.Errorf("failed to create tag redis: %v", err)
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

	h.logger.Infof("Tag with ID %s updated successfully", tagId)

	response := map[string]string{"message": "successful update"}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	go func() {
		if err = h.service.DeleteTagsRedis(r.Context(), tagId, userId); err != nil {
			h.logger.Errorf("failed to delete tag redis: %v", err)
			return
		}

		if err = h.service.UpdateTagsRedis(context.Background(), tagId, tagRequest, userId); err != nil {
			h.logger.Errorf("failed to update tag redis: %v", err)
		}
	}()

	return nil

}
