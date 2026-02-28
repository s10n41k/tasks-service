package handlers

import (
	"TODOLIST_Tasks/app/internal/apperror"
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/dto"
	"TODOLIST_Tasks/app/internal/tasks/port"
	"TODOLIST_Tasks/app/internal/tasks/service"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/api/resilience"
	"TODOLIST_Tasks/app/pkg/api/signature"
	sort2 "TODOLIST_Tasks/app/pkg/api/sort"
	logging2 "TODOLIST_Tasks/app/pkg/logging"
	"TODOLIST_Tasks/app/pkg/utils/CacheKey"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
)

const (
	taskURL        = "/tasks/:uuid"
	tasksByUserURL = "/v1/users/:userId/tasks"
	tasksByTags    = "/v1/users/:userId/tags/:tagsId/tasks"

	goroutineTimeout      = 5 * time.Second
	redisConcurrencyLimit = 500

	batchChannelSize = 10000
	batchMaxSize     = 500
	batchFlushMs     = 10

	deleteBatchChannelSize = 10000
	deleteBatchMaxSize     = 500
	deleteBatchFlushMs     = 10
)

type deleteItem struct {
	id     string
	userID string
}

type Handler struct {
	cmd           service.TaskCommandService
	query         service.TaskQueryService
	cache         service.TaskCacheService
	logger        *logging2.Logger
	redisSema     chan struct{}
	batchCh       chan domain.Task
	deleteBatchCh chan deleteItem
}

func NewHandler(cmd service.TaskCommandService, query service.TaskQueryService, cache service.TaskCacheService) *Handler {
	h := &Handler{
		cmd:           cmd,
		query:         query,
		cache:         cache,
		logger:        logging2.GetLogger().GetLoggerWithField("handler", "tasks"),
		redisSema:     make(chan struct{}, redisConcurrencyLimit),
		batchCh:       make(chan domain.Task, batchChannelSize),
		deleteBatchCh: make(chan deleteItem, deleteBatchChannelSize),
	}
	go h.startBatchWorker()
	go h.startDeleteBatchWorker()
	return h
}

// --- Маппинг DTO ↔ Domain ---

// dtoToEntity маппит входной DTO в доменную сущность.
func dtoToEntity(userID string, req dto.CreateTaskRequest) domain.Task {
	task := domain.Task{
		ID:          uuid.New().String(),
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		Status:      domain.NewStatus(req.Status),
		DueDate:     req.DueDate,
		UserID:      userID,
		TagName:     req.TagName,
	}
	if req.TagID != nil && *req.TagID != "" {
		task.TagID = req.TagID
	}
	return task
}

// entityToResponse маппит доменную сущность в HTTP-ответ.
func entityToResponse(t domain.Task) dto.TaskResponse {
	return dto.TaskResponse{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Priority:    t.Priority,
		Status:      string(t.Status),
		DueDate:     t.DueDate.Format("02-01-2006 15:04"),
		CreatedAt:   t.CreatedAt.Format("02-01-2006 15:04"),
		UserID:      t.UserID,
		TagName:     t.TagName,
	}
}

// updateRequestToPatch конвертирует HTTP-патч в port.UpdatePatch с доменными типами.
func updateRequestToPatch(req dto.UpdateTaskRequest) port.UpdatePatch {
	patch := port.UpdatePatch{
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		TagName:     req.TagName,
	}
	if req.Status != nil {
		s := domain.NewStatus(*req.Status)
		patch.Status = &s
	}
	if req.DueDate != nil {
		t := time.Time(*req.DueDate)
		patch.DueDate = &t
	}
	return patch
}

// --- Batch workers ---

func (h *Handler) startBatchWorker() {
	ticker := time.NewTicker(batchFlushMs * time.Millisecond)
	defer ticker.Stop()
	batch := make([]domain.Task, 0, batchMaxSize)

	for {
		select {
		case task := <-h.batchCh:
			batch = append(batch, task)
			if len(batch) >= batchMaxSize {
				h.flushBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				h.flushBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

// flushBatch сохраняет накопленные задачи в БД.
// Задачи с TagName вставляются по одной (резолв тега через CTE),
// остальные — одним батч-INSERT.
func (h *Handler) flushBatch(tasks []domain.Task) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	simple := tasks[:0:0]
	for _, t := range tasks {
		if strings.TrimSpace(t.TagName) != "" {
			if err := h.cmd.CreateTask(ctx, t); err != nil {
				h.logger.Errorf("flushBatch: CreateTask tagged %s: %v", t.ID, err)
			}
		} else {
			simple = append(simple, t)
		}
	}
	if len(simple) > 0 {
		if err := h.cmd.CreateTaskBatch(ctx, simple); err != nil {
			h.logger.Errorf("flushBatch: CreateTaskBatch %d tasks: %v", len(simple), err)
		}
	}

	for _, task := range tasks {
		t := task
		select {
		case h.redisSema <- struct{}{}:
			go func(t domain.Task) {
				defer func() { <-h.redisSema }()
				bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
				defer cancel()
				if err := h.cache.SetTask(bgCtx, t); err != nil {
					h.logger.Warnf("flushBatch: cache task %s: %v", t.ID, err)
				}
				_ = h.cache.InvalidateUserLists(bgCtx, t.UserID)
			}(t)
		default:
			h.logger.Warnf("redis semaphore full, skip cache for task %s", t.ID)
		}
	}
}

func (h *Handler) startDeleteBatchWorker() {
	ticker := time.NewTicker(deleteBatchFlushMs * time.Millisecond)
	defer ticker.Stop()
	batch := make([]deleteItem, 0, deleteBatchMaxSize)

	for {
		select {
		case item := <-h.deleteBatchCh:
			batch = append(batch, item)
			if len(batch) >= deleteBatchMaxSize {
				h.flushDeleteBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				h.flushDeleteBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

func (h *Handler) flushDeleteBatch(items []deleteItem) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.id
	}
	if err := h.cmd.DeleteTaskBatch(ctx, ids); err != nil {
		h.logger.Errorf("flushDeleteBatch: %d tasks: %v", len(ids), err)
	}

	for _, item := range items {
		it := item
		select {
		case h.redisSema <- struct{}{}:
			go func(id, userID string) {
				defer func() { <-h.redisSema }()
				bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
				defer cancel()
				_ = h.cache.DeleteCachedTask(bgCtx, id)
				if userID != "" {
					_ = h.cache.InvalidateUserLists(bgCtx, userID)
				}
			}(it.id, it.userID)
		default:
			h.logger.Warnf("redis semaphore full, skip cache delete for task %s", it.id)
		}
	}
}

// --- HTTP Handlers ---

func (h *Handler) Register(router *httprouter.Router) {
	router.HandlerFunc(http.MethodPost, tasksByUserURL, resilience.WriteMiddleware(apperror.Middleware(h.Create)))
	router.HandlerFunc(http.MethodPatch, taskURL, signature.Middleware(resilience.WriteMiddleware(apperror.Middleware(h.Update))))
	router.HandlerFunc(http.MethodGet, taskURL, signature.Middleware(resilience.Middleware(apperror.Middleware(h.FindOne))))
	router.HandlerFunc(http.MethodGet, tasksByUserURL, signature.Middleware(resilience.Middleware(filter.Middleware(sort2.MiddleWare(apperror.Middleware(h.GetList), "due_date", sort2.ASC)))))
	router.HandlerFunc(http.MethodDelete, taskURL, signature.Middleware(apperror.Middleware(h.Delete)))
	router.HandlerFunc(http.MethodGet, tasksByTags, signature.Middleware(resilience.Middleware(filter.Middleware(sort2.MiddleWare(apperror.Middleware(h.GetListByTag), "due_date", sort2.ASC)))))
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	userID := params.ByName("userId")
	if userID == "" {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return err
	}
	defer r.Body.Close()

	var req dto.CreateTaskRequest
	if err := req.UnmarshalJSON(body); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return nil
	}

	task := dtoToEntity(userID, req)

	select {
	case h.batchCh <- task:
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(fmt.Sprintf("Task accepted with ID: %s", task.ID)))
	default:
		h.logger.Warnf("batch channel full, fallback to sync for task %s", task.ID)
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if err := h.cmd.CreateTask(ctx, task); err != nil {
			h.logger.Errorf("Create: sync fallback %s: %v", task.ID, err)
			http.Error(w, "Failed to save task", http.StatusInternalServerError)
			return nil
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(fmt.Sprintf("Task created with ID: %s", task.ID)))
	}
	return nil
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	id := params.ByName("uuid")
	if id == "" {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return errors.New("invalid ID")
	}

	var req dto.UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Errorf("Update: decode body for task %s: %v", id, err)
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	patch := updateRequestToPatch(req)
	task, err := h.cmd.UpdateTask(ctx, id, patch)
	if err != nil {
		h.logger.Errorf("Update: service UpdateTask %s: %v", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "successful update"})

	select {
	case h.redisSema <- struct{}{}:
		go func(t domain.Task) {
			defer func() { <-h.redisSema }()
			bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
			defer cancel()
			_ = h.cache.SetTask(bgCtx, t)
			_ = h.cache.InvalidateUserLists(bgCtx, t.UserID)
		}(task)
	default:
		h.logger.Warnf("redis semaphore full, skip async cache update for task %s", task.ID)
	}
	return nil
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	id := params.ByName("uuid")
	if id == "" {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Берём userID из кэша до удаления — нужен для инвалидации списков.
	cachedTask, _ := h.cache.GetTask(ctx, id)
	item := deleteItem{id: id, userID: cachedTask.UserID}

	select {
	case h.deleteBatchCh <- item:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"message":"delete accepted"}`))
	default:
		h.logger.Warnf("delete batch full, fallback to sync delete for task %s", id)
		if err := h.cmd.DeleteTask(ctx, id); err != nil {
			h.logger.Errorf("Delete: service DeleteTask %s: %v", id, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return nil
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "successful delete"})

		select {
		case h.redisSema <- struct{}{}:
			go func() {
				defer func() { <-h.redisSema }()
				bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
				defer cancel()
				_ = h.cache.DeleteCachedTask(bgCtx, id)
				if cachedTask.UserID != "" {
					_ = h.cache.InvalidateUserLists(bgCtx, cachedTask.UserID)
				}
			}()
		default:
			h.logger.Warnf("redis semaphore full, skip async cache delete for task %s", id)
		}
	}
	return nil
}

func (h *Handler) FindOne(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	id := params.ByName("uuid")
	if id == "" {
		http.Error(w, "Identifier is required", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// cache-first: service.FindTask уже пробует кэш
	task, err := h.query.FindTask(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Task not found", http.StatusNotFound)
			return nil
		}
		h.logger.Errorf("FindOne: task %s: %v", id, err)
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return nil
	}

	// Асинхронно кэшируем после DB miss (service.FindTask не кэширует сам).
	select {
	case h.redisSema <- struct{}{}:
		go func(t domain.Task) {
			defer func() { <-h.redisSema }()
			bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
			defer cancel()
			if err := h.cache.SetTask(bgCtx, t); err != nil {
				h.logger.Warnf("FindOne: cache task %s: %v", t.ID, err)
			}
		}(task)
	default:
		h.logger.Warnf("redis semaphore full, skip cache for task %s on FindOne", task.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(entityToResponse(task)); err != nil {
		h.logger.Errorf("FindOne: encode response: %v", err)
		return err
	}
	return nil
}

func (h *Handler) GetList(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	userID := params.ByName("userId")
	if userID == "" {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return nil
	}

	var filterOpts filter.Option
	if f, ok := r.Context().Value(filter.OptionsContextKey).([]filter.Field); ok {
		filterOpts.Fields = f
	}
	var sortOpts sort2.Options
	if s, ok := r.Context().Value(sort2.OptionsContextKey).(sort2.Options); ok {
		sortOpts = s
	}

	cacheKey := CacheKey.BuildCacheKey(userID, filterOpts, sortOpts)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	tasks, err := h.cache.GetList(ctx, cacheKey)
	if err != nil || len(tasks) == 0 {
		tasks, err = h.query.FindTasksByUser(ctx, userID, sortOpts, filterOpts)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				w.WriteHeader(http.StatusNoContent)
				return nil
			}
			h.logger.Errorf("GetList: FindTasksByUser user %s: %v", userID, err)
			http.Error(w, "Error finding tasks", http.StatusInternalServerError)
			return nil
		}
		if len(tasks) > 0 {
			go func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
				defer cancel()
				_ = h.cache.SetList(bgCtx, cacheKey, tasks)
			}()
		}
	}

	if len(tasks) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}

	responses := make([]dto.TaskResponse, 0, len(tasks))
	for _, t := range tasks {
		responses = append(responses, entityToResponse(t))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		h.logger.Errorf("GetList: encode response: %v", err)
		return err
	}
	return nil
}

func (h *Handler) GetListByTag(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	userID := params.ByName("userId")
	tagID := params.ByName("tagsId")
	if userID == "" || tagID == "" {
		http.Error(w, "Invalid user ID or tag ID", http.StatusBadRequest)
		return nil
	}

	var filterOpts filter.Option
	if f, ok := r.Context().Value(filter.OptionsContextKey).([]filter.Field); ok {
		filterOpts.Fields = f
	}
	var sortOpts sort2.Options
	if s, ok := r.Context().Value(sort2.OptionsContextKey).(sort2.Options); ok {
		sortOpts = s
	}

	cacheKey := CacheKey.BuildCacheKeyWithTag(userID, tagID, filterOpts, sortOpts)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	tasks, err := h.cache.GetList(ctx, cacheKey)
	if err != nil || len(tasks) == 0 {
		tasks, err = h.query.FindTasksByTag(ctx, userID, tagID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				w.WriteHeader(http.StatusNoContent)
				return nil
			}
			h.logger.Errorf("GetListByTag: user %s tag %s: %v", userID, tagID, err)
			http.Error(w, "Error finding tasks", http.StatusInternalServerError)
			return nil
		}
		if len(tasks) > 0 {
			go func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
				defer cancel()
				_ = h.cache.SetList(bgCtx, cacheKey, tasks)
			}()
		}
	}

	if len(tasks) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}

	responses := make([]dto.TaskResponse, 0, len(tasks))
	for _, t := range tasks {
		responses = append(responses, entityToResponse(t))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		h.logger.Errorf("GetListByTag: encode response: %v", err)
		return err
	}
	return nil
}
