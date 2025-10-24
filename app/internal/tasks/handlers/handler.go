package handlers

import (
	"TODOLIST_Tasks/app/internal/apperror"
	model2 "TODOLIST_Tasks/app/internal/tasks/model"
	"TODOLIST_Tasks/app/internal/tasks/service"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/api/resilience"
	sort2 "TODOLIST_Tasks/app/pkg/api/sort"
	logging2 "TODOLIST_Tasks/app/pkg/logging"
	"TODOLIST_Tasks/app/pkg/utils/CacheKey"
	"TODOLIST_Tasks/app/pkg/utils/translator"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
	"io"
	"net/http"
	"time"
)

const (
	taskURL        = "/tasks/:uuid"
	tasksByUserURL = "/v1/users/:userId/tasks"
	tasksByTags    = "/v1/users/:userId/tags/:tagsId/tasks"
)

type Handler struct {
	service *service.Service
	logger  *logging2.Logger
}

func NewHandler(service *service.Service) *Handler {
	return &Handler{
		service: service,
		logger:  logging2.GetLogger().GetLoggerWithField("handler", "tasks"),
	}
}

func (h *Handler) Register(router *httprouter.Router) {
	router.HandlerFunc(http.MethodPost, tasksByUserURL, apperror.Middleware(h.Create))
	router.HandlerFunc(http.MethodPatch, taskURL, apperror.Middleware(h.Update))
	router.HandlerFunc(http.MethodGet, taskURL, resilience.Middleware(apperror.Middleware(h.FindOne)))
	router.HandlerFunc(http.MethodGet, tasksByUserURL, resilience.Middleware(filter.Middleware(sort2.MiddleWare(apperror.Middleware(h.GetList), "due_date", sort2.ASC))))
	router.HandlerFunc(http.MethodDelete, taskURL, apperror.Middleware(h.Delete))
	router.HandlerFunc(http.MethodGet, tasksByTags, resilience.Middleware(filter.Middleware(sort2.MiddleWare(apperror.Middleware(h.GetListByTag), "due_date", sort2.ASC))))
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) error {
	h.logger.Info("CreateTask called")

	params := httprouter.ParamsFromContext(r.Context())
	userId := params.ByName("userId")
	if userId == "" {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return err
	}
	defer r.Body.Close()

	var dto model2.TaskCreateDTO
	if err := dto.UnmarshalJSON(body); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return err
	}

	task := model2.Task{
		Id:          uuid.New().String(),
		Title:       dto.Title,
		Description: dto.Description,
		Priory:      dto.Priory,
		Status:      dto.Status,
		DueDate:     dto.DueDate,
		UserID:      userId,
		TagID:       dto.TagID,
		TagsName:    dto.TagName,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	taskResult, err := h.service.CreateTask(ctx, task)
	if err != nil {
		http.Error(w, "Failed to save task", http.StatusInternalServerError)
		return err
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(fmt.Sprintf("Task created with ID: %s", taskResult.Id)))

	go func(taskResult model2.Task) {
		_ = h.service.CreateTaskRedis(context.Background(), taskResult)
	}(taskResult)

	return nil
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) error {
	h.logger.Info("UpdateTask called")

	params := httprouter.ParamsFromContext(r.Context())
	id := params.ByName("uuid")
	if id == "" {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return errors.New("invalid ID")
	}

	var dto model2.TaskUpdateDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return err
	}

	ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
	defer cancel()

	task, err := h.service.UpdateTask(ctx, id, dto)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "successful update"})

	go func(t model2.Task) {
		_ = h.service.UpdateTaskRedis(context.Background(), t)
	}(task)

	return nil
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) error {
	h.logger.Info("DeleteTask called")

	params := httprouter.ParamsFromContext(r.Context())
	id := params.ByName("uuid")
	if id == "" {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
	defer cancel()

	_, err := h.service.DeleteTask(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "successful delete"})

	go func(id string) {
		_ = h.service.DeleteTaskRedis(context.Background(), id)
	}(id)

	return nil
}

func (h *Handler) FindOne(w http.ResponseWriter, r *http.Request) error {
	h.logger.Info("FindOne called")

	// --- Извлекаем id из URL ---
	params := httprouter.ParamsFromContext(r.Context())
	id := params.ByName("uuid")
	if id == "" {
		http.Error(w, "Identifier is required", http.StatusBadRequest)
		return nil
	}

	// --- Контекст с таймаутом ---
	ctx, cancel := context.WithTimeout(r.Context(), 350*time.Millisecond)
	defer cancel()

	// --- Пытаемся достать задачу ---
	task, err := h.service.FindOneRedis(ctx, id)
	if err != nil || task.Id == "" {
		h.logger.Infof("Cache miss for task %s, querying DB...", id)
		task, err = h.service.FindOneTask(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Task not found", http.StatusNotFound)
				return nil
			}
			h.logger.Errorf("Failed to get task %s: %v", id, err)
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return err
		}
	}

	// --- Успешный ответ ---
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(translator.ToTaskResponse(task)); err != nil {
		h.logger.Errorf("Failed to write response: %v", err)
		return err
	}

	go func(t model2.Task) {
		if err := h.service.CreateTaskRedis(context.Background(), t); err != nil {
			h.logger.Warnf("Failed to cache task %s in Redis: %v", t.Id, err)
		}
	}(task)

	return nil
}

func (h *Handler) GetList(w http.ResponseWriter, r *http.Request) error {
	h.logger.Info("GetList called")

	params := httprouter.ParamsFromContext(r.Context())
	userId := params.ByName("userId")
	if userId == "" {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return nil
	}

	var filterOptions filter.Option
	if fOptions, ok := r.Context().Value(filter.OptionsContextKey).([]filter.Field); ok {
		filterOptions.Fields = fOptions
	}

	var sortOptions sort2.Options
	if sOptions, ok := r.Context().Value(sort2.OptionsContextKey).(sort2.Options); ok {
		sortOptions = sOptions
	}

	cacheKey := CacheKey.BuildCacheKey(userId, filterOptions, sortOptions)

	ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
	defer cancel()

	// --- Попытка получить задачи из кэша ---
	tasks, err := h.service.GetTasksFromCache(ctx, cacheKey)
	if err != nil || len(tasks) == 0 {
		// Если кэш пуст или произошла ошибка — идем в базу
		tasks, err = h.service.FindAllTasks(ctx, sortOptions, filterOptions, userId)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				w.WriteHeader(http.StatusNoContent)
				return nil
			}
			http.Error(w, "Error finding tasks", http.StatusInternalServerError)
			return err
		}
	}

	if len(tasks) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}

	responses := make([]model2.TaskResponse, 0, len(tasks))
	for _, t := range tasks {
		responses = append(responses, translator.ToTaskResponse(t))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		h.logger.Errorf("Failed to encode response: %v", err)
		return err
	}

	go func() {
		_ = h.service.SetTasksToCache(context.Background(), cacheKey, tasks)
	}()

	return nil
}

func (h *Handler) GetListByTag(w http.ResponseWriter, r *http.Request) error {
	h.logger.Info("GetListByTag called")

	params := httprouter.ParamsFromContext(r.Context())
	userId := params.ByName("userId")
	tagId := params.ByName("tagsId")
	if userId == "" || tagId == "" {
		http.Error(w, "Invalid user ID or tag ID", http.StatusBadRequest)
		return nil
	}

	var filterOptions filter.Option
	if fOptions, ok := r.Context().Value(filter.OptionsContextKey).([]filter.Field); ok {
		filterOptions.Fields = fOptions
	}

	var sortOptions sort2.Options
	if sOptions, ok := r.Context().Value(sort2.OptionsContextKey).(sort2.Options); ok {
		sortOptions = sOptions
	}

	cacheKey := CacheKey.BuildCacheKeyWithTag(userId, tagId, filterOptions, sortOptions)

	ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
	defer cancel()

	// --- Попытка получить задачи из кэша ---
	tasks, err := h.service.GetTasksFromCache(ctx, cacheKey)
	if err != nil || len(tasks) == 0 {
		// Если кэш пуст или произошла ошибка — идем в базу
		tasks, err = h.service.FindAllByTag(ctx, userId, tagId)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				w.WriteHeader(http.StatusNoContent)
				return nil
			}
			http.Error(w, "Error finding tasks", http.StatusInternalServerError)
			return err
		}
	}

	if len(tasks) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}

	responses := make([]model2.TaskResponse, 0, len(tasks))
	for _, t := range tasks {
		responses = append(responses, translator.ToTaskResponse(t))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		h.logger.Errorf("Failed to encode response: %v", err)
		return err
	}

	go func() {
		_ = h.service.SetTasksToCache(context.Background(), cacheKey, tasks)
	}()

	return nil

}
