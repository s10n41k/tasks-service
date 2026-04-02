package handlers

import (
	"TODOLIST_Tasks/app/internal/apperror"
	taskBatch "TODOLIST_Tasks/app/internal/tasks/batch"
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/dto"
	"TODOLIST_Tasks/app/internal/tasks/model"
	"TODOLIST_Tasks/app/internal/tasks/port"
	"TODOLIST_Tasks/app/internal/tasks/service"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/api/resilience"
	"TODOLIST_Tasks/app/pkg/api/signature"
	sort2 "TODOLIST_Tasks/app/pkg/api/sort"
	logging2 "TODOLIST_Tasks/app/pkg/logging"
	"TODOLIST_Tasks/app/pkg/utils/CacheKey"
	"TODOLIST_Tasks/app/pkg/utils/validate"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
)

// syncSubscriptionRequest — тело запроса для синхронизации подписки.
type syncSubscriptionRequest struct {
	UserID          string     `json:"user_id"`
	Name            string     `json:"name,omitempty"`
	HasSubscription bool       `json:"has_subscription"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	TelegramChatID  *int64     `json:"telegram_chat_id,omitempty"`
}

const (
	tasksURL    = "/tasks"
	taskURL     = "/tasks/:uuid"
	tasksByTags = "/v1/users/:userId/tags/:tagsId/tasks"
	tasksByUser = "/v1/users/:userId/tasks"

	// Подзадачи обычных задач
	taskSubtasksURL = "/v1/users/:userId/tasks/:uuid/subtasks"
	taskSubtaskURL  = "/v1/users/:userId/tasks/:uuid/subtasks/:sid"

	subURL = "/internal/subscriptions/sync"

	// Admin endpoints
	adminTasksURL = "/admin/tasks"

	goroutineTimeout      = 5 * time.Second
	redisConcurrencyLimit = 500
)

type Handler struct {
	cmd        service.TaskCommandService
	query      service.TaskQueryService
	cache      service.TaskCacheService
	subtaskSvc service.SubtaskService
	adminSvc   service.TaskAdminService
	subSvc     service.SubscriptionService
	wsNotifier port.WsNotifier
	batch      *taskBatch.Processor
	logger     *logging2.Logger
	redisSema  chan struct{}
}

func NewHandler(
	cmd service.TaskCommandService,
	query service.TaskQueryService,
	cache service.TaskCacheService,
	subtaskSvc service.SubtaskService,
	subSvc service.SubscriptionService,
	adminSvc service.TaskAdminService,
	batchProc *taskBatch.Processor,
	wsNotifier port.WsNotifier,
) *Handler {
	return &Handler{
		cmd:        cmd,
		query:      query,
		cache:      cache,
		subtaskSvc: subtaskSvc,
		adminSvc:   adminSvc,
		subSvc:     subSvc,
		wsNotifier: wsNotifier,
		batch:      batchProc,
		logger:     logging2.GetLogger().GetLoggerWithField("handler", "tasks"),
		redisSema:  make(chan struct{}, redisConcurrencyLimit),
	}
}

// --- Маппинг DTO ↔ Domain ---

// dtoToEntity маппит входной DTO в доменную сущность.
func dtoToEntity(userID string, req dto.CreateTaskRequest) domain.Task {
	task := domain.Task{
		ID:          uuid.New().String(),
		Title:       req.Title,
		Description: req.Description,
		Priority:    domain.NewPriory(req.Priority),
		Status:      domain.NewStatus(req.Status),
		DueDate:     req.DueDate,
		UserID:      userID,
		TagName:     req.TagName,
	}
	if req.TagID != nil && *req.TagID != "" {
		task.TagID = req.TagID
	}
	for i, s := range req.Subtasks {
		task.Subtasks = append(task.Subtasks, domain.Subtask{
			Title: s.Title,
			Order: i,
		})
	}
	return task
}

// entityToResponse маппит доменную сущность в HTTP-ответ.
func entityToResponse(t domain.Task) dto.TaskResponse {
	resp := dto.TaskResponse{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Priority:    string(t.Priority),
		Status:      string(t.Status),
		DueDate:     t.DueDate.Format("02-01-2006 15:04"),
		CreatedAt:   t.CreatedAt.Format("02-01-2006 15:04"),
		UserID:      t.UserID,
		TagName:     t.TagName,
	}
	for _, s := range t.Subtasks {
		resp.Subtasks = append(resp.Subtasks, dto.SubtaskResponse{
			ID:     s.ID,
			Title:  s.Title,
			IsDone: s.IsDone,
			Order:  s.Order,
		})
	}
	return resp
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

// --- Cache helpers ---

// invalidateTaskCache синхронно инвалидирует кэш задачи и список пользователя.
func (h *Handler) invalidateTaskCache(ctx context.Context, taskID, userID string) {
	_ = h.cache.DeleteCachedTask(ctx, taskID)
	if userID != "" {
		_ = h.cache.InvalidateUserLists(ctx, userID)
	}
}

// invalidateTaskCacheAsync асинхронно инвалидирует кэш с учётом семафора параллелизма.
func (h *Handler) invalidateTaskCacheAsync(taskID, userID string) {
	select {
	case h.redisSema <- struct{}{}:
		go func() {
			defer func() { <-h.redisSema }()
			bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
			defer cancel()
			_ = h.cache.DeleteCachedTask(bgCtx, taskID)
			if userID != "" {
				_ = h.cache.InvalidateUserLists(bgCtx, userID)
			}
		}()
	default:
		h.logger.Warnf("redis semaphore full, skip async cache invalidation for task %s", taskID)
	}
}

// --- HTTP Handlers ---

func (h *Handler) Register(router *httprouter.Router) {
	router.HandlerFunc(http.MethodPost, tasksURL, signature.Middleware(resilience.WriteMiddleware(apperror.Middleware(h.Create))))
	router.HandlerFunc(http.MethodPost, tasksByUser, signature.Middleware(resilience.WriteMiddleware(apperror.Middleware(h.Create))))
	router.HandlerFunc(http.MethodPatch, taskURL, signature.Middleware(resilience.WriteMiddleware(apperror.Middleware(h.Update))))
	router.HandlerFunc(http.MethodGet, taskURL, signature.Middleware(resilience.Middleware(apperror.Middleware(h.FindOne))))
	router.HandlerFunc(http.MethodGet, tasksURL, signature.Middleware(resilience.Middleware(filter.Middleware(sort2.MiddleWare(apperror.Middleware(h.GetList), "due_date", sort2.ASC)))))
	router.HandlerFunc(http.MethodGet, tasksByUser, signature.Middleware(resilience.Middleware(filter.Middleware(sort2.MiddleWare(apperror.Middleware(h.GetList), "due_date", sort2.ASC)))))
	router.HandlerFunc(http.MethodDelete, taskURL, signature.Middleware(apperror.Middleware(h.Delete)))
	router.HandlerFunc(http.MethodGet, tasksByTags, signature.Middleware(resilience.Middleware(filter.Middleware(sort2.MiddleWare(apperror.Middleware(h.GetListByTag), "due_date", sort2.ASC)))))
	router.HandlerFunc(http.MethodPost, subURL, signature.Middleware(apperror.Middleware(h.SyncSubscription)))

	// Подзадачи обычных задач: create / toggle / update title / delete
	router.HandlerFunc(http.MethodPost, taskSubtasksURL, signature.Middleware(apperror.Middleware(h.AddSubtask)))
	router.HandlerFunc(http.MethodPatch, taskSubtaskURL, signature.Middleware(apperror.Middleware(h.ToggleTaskSubtask)))
	router.HandlerFunc(http.MethodPut, taskSubtaskURL, signature.Middleware(apperror.Middleware(h.UpdateTaskSubtask)))
	router.HandlerFunc(http.MethodDelete, taskSubtaskURL, signature.Middleware(apperror.Middleware(h.DeleteTaskSubtask)))

	// Admin endpoints
	router.HandlerFunc(http.MethodGet, adminTasksURL, signature.Middleware(apperror.Middleware(h.AdminGetAllTasks)))
	router.HandlerFunc(http.MethodDelete, "/admin/tasks/:uuid", signature.Middleware(apperror.Middleware(h.AdminDeleteTask)))
	router.HandlerFunc(http.MethodDelete, "/admin/shared-tasks/:uuid", signature.Middleware(apperror.Middleware(h.AdminDeleteSharedTask)))
	router.HandlerFunc(http.MethodPatch, "/admin/tasks/:uuid/restore", signature.Middleware(apperror.Middleware(h.AdminRestoreTask)))
	router.HandlerFunc(http.MethodPatch, "/admin/shared-tasks/:uuid/restore", signature.Middleware(apperror.Middleware(h.AdminRestoreSharedTask)))

	// Подтверждение удаления задачи пользователем
	router.HandlerFunc(http.MethodPost, "/v1/users/:userId/tasks/:uuid/acknowledge-admin-deletion",
		signature.Middleware(apperror.Middleware(h.AcknowledgeAdminDeletion)))
	router.HandlerFunc(http.MethodPost, "/v1/users/:userId/shared-tasks/:stid/acknowledge-admin-deletion",
		signature.Middleware(apperror.Middleware(h.AcknowledgeAdminDeletionShared)))
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) error {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil
	}
	if err := validate.UUID(userID); err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return nil
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return err
	}

	var req dto.CreateTaskRequest
	if err := req.UnmarshalJSON(body); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return nil
	}

	task := dtoToEntity(userID, req)

	// Проверяем лимит активных задач
	{
		limitCtx, limitCancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer limitCancel()
		count, err := h.query.CountActiveTasks(limitCtx, userID)
		if err != nil {
			h.logger.Errorf("Create: count active tasks user %s: %v", userID, err)
			http.Error(w, "Failed to check task limit", http.StatusInternalServerError)
			return nil
		}
		if count >= domain.MaxActiveTasks {
			http.Error(w, "Достигнут лимит активных задач (100). Завершите некоторые задачи.", http.StatusUnprocessableEntity)
			return nil
		}
	}

	// Задачи с подзадачами вставляются синхронно (нельзя через batch — subtasks нужно вставить атомарно)
	if task.HasSubtasks() {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if err := h.cmd.CreateTask(ctx, task); err != nil {
			h.logger.Errorf("Create: with subtasks %s: %v", task.ID, err)
			http.Error(w, "Failed to save task", http.StatusInternalServerError)
			return nil
		}
		go func(t domain.Task) {
			bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
			defer cancel()
			_ = h.cache.SetTask(bgCtx, t)
			_ = h.cache.InvalidateUserLists(bgCtx, t.UserID)
		}(task)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(fmt.Sprintf("Task created with ID: %s", task.ID)))
		return nil
	}

	if h.batch.EnqueueCreate(task) {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(fmt.Sprintf("Task accepted with ID: %s", task.ID)))
	} else {
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
	if err := validate.UUID(id); err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return nil
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(entityToResponse(task))

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
	if err := validate.UUID(id); err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Берём userID из кэша до удаления — нужен для инвалидации списков.
	cachedTask, _ := h.cache.GetTask(ctx, id)
	userID := cachedTask.UserID
	if userID == "" {
		if dbTask, _, dbErr := h.query.FindTask(ctx, id); dbErr == nil {
			userID = dbTask.UserID
		}
	}

	if err := h.cmd.DeleteTask(ctx, id); err != nil {
		h.logger.Errorf("Delete: service DeleteTask %s: %v", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}

	h.invalidateTaskCache(ctx, id, userID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "successful delete"})
	return nil
}

func (h *Handler) FindOne(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	id := params.ByName("uuid")
	if err := validate.UUID(id); err != nil {
		http.Error(w, "Identifier is required", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	task, fromCache, err := h.query.FindTask(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Task not found", http.StatusNotFound)
			return nil
		}
		h.logger.Errorf("FindOne: task %s: %v", id, err)
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return nil
	}

	if !fromCache {
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
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil
	}
	if err := validate.UUID(userID); err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(tasks); err != nil {
		h.logger.Errorf("GetList: encode response: %v", err)
		return err
	}
	return nil
}

// SyncSubscription — внутренний эндпоинт для синхронизации данных подписки.
func (h *Handler) SyncSubscription(w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return nil
	}
	defer r.Body.Close()

	var req syncSubscriptionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return nil
	}
	if req.UserID == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return nil
	}

	sub := port.UserSubscription{
		UserID:          req.UserID,
		Name:            req.Name,
		HasSubscription: req.HasSubscription,
		ExpiresAt:       req.ExpiresAt,
		TelegramChatID:  req.TelegramChatID,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.subSvc.SyncSubscription(ctx, sub); err != nil {
		h.logger.Errorf("SyncSubscription user %s: %v", req.UserID, err)
		http.Error(w, "failed to sync subscription", http.StatusInternalServerError)
		return nil
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"ok"}`))
	return nil
}

// AddSubtask — POST /v1/users/:userId/tasks/:uuid/subtasks
// Добавляет подзадачу к существующей задаче. Только владелец задачи может добавлять подзадачи.
func (h *Handler) AddSubtask(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	taskID := params.ByName("uuid")
	userID := r.Header.Get("X-User-ID")
	if err := validate.UUID(taskID); err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return nil
	}
	if err := validate.UUID(userID); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil
	}

	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		http.Error(w, "title обязателен", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	subtask, err := h.subtaskSvc.AddSubtask(ctx, taskID, userID, body.Title)
	if err != nil {
		h.logger.Errorf("AddSubtask task %s user %s: %v", taskID, userID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}

	h.invalidateTaskCacheAsync(taskID, userID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	return json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      subtask.ID,
		"title":   subtask.Title,
		"is_done": subtask.IsDone,
		"order":   subtask.Order,
	})
}

// ToggleTaskSubtask — PATCH /v1/users/:userId/tasks/:uuid/subtasks/:sid
// Переключает выполненность подзадачи обычной задачи.
// Если все подзадачи выполнены — задача автоматически переходит в статус "completed".
func (h *Handler) ToggleTaskSubtask(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	subtaskID := params.ByName("sid")
	userID := r.Header.Get("X-User-ID")
	if err := validate.UUID(subtaskID); err != nil {
		http.Error(w, "invalid subtask id", http.StatusBadRequest)
		return nil
	}
	if err := validate.UUID(userID); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	subtask, err := h.subtaskSvc.ToggleSubtaskDone(ctx, subtaskID, userID)
	if err != nil {
		h.logger.Errorf("ToggleTaskSubtask subtask %s user %s: %v", subtaskID, userID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}

	h.invalidateTaskCacheAsync(subtask.TaskID, userID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      subtask.ID,
		"is_done": subtask.IsDone,
	})
}

// UpdateTaskSubtask — PUT /v1/users/:userId/tasks/:uuid/subtasks/:sid
// Переименовывает подзадачу. Body: {"title": "..."}
func (h *Handler) UpdateTaskSubtask(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	subtaskID := params.ByName("sid")
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil
	}

	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		http.Error(w, "title обязателен", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.subtaskSvc.UpdateSubtask(ctx, subtaskID, userID, body.Title); err != nil {
		h.logger.Errorf("UpdateTaskSubtask subtask %s user %s: %v", subtaskID, userID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

// DeleteTaskSubtask — DELETE /v1/users/:userId/tasks/:uuid/subtasks/:sid
func (h *Handler) DeleteTaskSubtask(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	subtaskID := params.ByName("sid")
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.subtaskSvc.DeleteSubtask(ctx, subtaskID, userID); err != nil {
		h.logger.Errorf("DeleteTaskSubtask subtask %s user %s: %v", subtaskID, userID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) GetListByTag(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	tagID := params.ByName("tagsId")
	userID := r.Header.Get("X-User-ID")
	if userID == "" || tagID == "" {
		http.Error(w, "unauthorized or missing tag ID", http.StatusUnauthorized)
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(tasks); err != nil {
		h.logger.Errorf("GetListByTag: encode response: %v", err)
		return err
	}
	return nil
}

// AdminDeleteTask выполняет soft-delete задачи: помечает как удалённую администратором.
// Пользователь увидит задачу с пометкой и должен подтвердить удаление.
func (h *Handler) AdminDeleteTask(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-User-Role") != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}

	params := httprouter.ParamsFromContext(r.Context())
	id := params.ByName("uuid")
	if id == "" {
		http.Error(w, "task id required", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Получаем задачу до мягкого удаления — нужен title для WS-уведомления
	existingTask, _, _ := h.query.FindTask(ctx, id)

	// Soft-delete: помечаем задачу
	ownerID, err := h.adminSvc.AdminSoftDelete(ctx, id)
	if err != nil {
		h.logger.Errorf("AdminDeleteTask soft-delete %s: %v", id, err)
		http.Error(w, "task not found or error", http.StatusNotFound)
		return nil
	}

	// Синхронно инвалидируем кэш ДО WS-уведомления:
	// если фронт получит событие до сброса кэша — увидит устаревшие данные.
	h.invalidateTaskCache(ctx, id, ownerID)

	// Асинхронно уведомляем владельца через WS
	if ownerID != "" && h.wsNotifier != nil {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
			defer cancel()
			_ = h.wsNotifier.Notify(bgCtx, ownerID, "task_admin_deleted", map[string]string{
				"task_id": id,
				"title":   existingTask.Title,
			})
		}()
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// AdminDeleteSharedTask выполняет soft-delete совместной задачи.
func (h *Handler) AdminDeleteSharedTask(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-User-Role") != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}

	params := httprouter.ParamsFromContext(r.Context())
	id := params.ByName("uuid")
	if id == "" {
		http.Error(w, "shared task id required", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	proposerID, addresseeID, title, err := h.adminSvc.AdminSoftDeleteShared(ctx, id)
	if err != nil {
		h.logger.Errorf("AdminDeleteSharedTask soft-delete %s: %v", id, err)
		http.Error(w, "shared task not found or error", http.StatusNotFound)
		return nil
	}

	// Синхронно инвалидируем кэш списков обоих участников ДО WS-уведомления
	if proposerID != "" {
		_ = h.cache.InvalidateUserLists(ctx, proposerID)
	}
	if addresseeID != "" && addresseeID != proposerID {
		_ = h.cache.InvalidateUserLists(ctx, addresseeID)
	}

	// Уведомляем обоих участников с title
	if h.wsNotifier != nil {
		payload := map[string]string{"shared_task_id": id, "title": title}
		if proposerID != "" {
			go func(uid string) {
				bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
				defer cancel()
				_ = h.wsNotifier.Notify(bgCtx, uid, "shared_task_admin_deleted", payload)
			}(proposerID)
		}
		if addresseeID != "" && addresseeID != proposerID {
			go func(uid string) {
				bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
				defer cancel()
				_ = h.wsNotifier.Notify(bgCtx, uid, "shared_task_admin_deleted", payload)
			}(addresseeID)
		}
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// AcknowledgeAdminDeletion — пользователь подтверждает удаление задачи → физически удаляем.
func (h *Handler) AcknowledgeAdminDeletion(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	taskID := params.ByName("uuid")
	userID := r.Header.Get("X-User-ID")
	if taskID == "" || userID == "" {
		http.Error(w, "task id and user id required", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.adminSvc.AcknowledgeAdminDeletion(ctx, taskID, userID); err != nil {
		h.logger.Errorf("AcknowledgeAdminDeletion task %s user %s: %v", taskID, userID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}

	// Инвалидируем кэш синхронно — иначе loadTasks сразу после ответа попадёт в старый кэш
	h.invalidateTaskCache(ctx, taskID, userID)

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// AcknowledgeAdminDeletionShared — пользователь подтверждает удаление совместной задачи.
func (h *Handler) AcknowledgeAdminDeletionShared(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	taskID := params.ByName("uuid")
	userID := r.Header.Get("X-User-ID")
	if taskID == "" || userID == "" {
		http.Error(w, "task id and user id required", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.adminSvc.AcknowledgeAdminDeletionShared(ctx, taskID, userID); err != nil {
		h.logger.Errorf("AcknowledgeAdminDeletionShared task %s user %s: %v", taskID, userID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}

	// Инвалидируем кэш синхронно
	h.invalidateTaskCache(ctx, taskID, userID)

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// AdminRestoreTask — восстанавливает задачу, удалённую администратором (admin_deleted → FALSE).
func (h *Handler) AdminRestoreTask(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	id := params.ByName("uuid")
	if id == "" {
		http.Error(w, "task id required", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	ownerID, err := h.adminSvc.AdminRestore(ctx, id)
	if err != nil {
		h.logger.Errorf("AdminRestoreTask %s: %v", id, err)
		if errors.Is(err, port.ErrNotFound) {
			http.Error(w, "задача не найдена", http.StatusNotFound)
		} else if errors.Is(err, port.ErrDeadlineExpired) {
			http.Error(w, "срок задачи истёк, восстановление невозможно", http.StatusUnprocessableEntity)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}

	// Синхронно инвалидируем кэш ДО WS-уведомления
	h.invalidateTaskCache(ctx, id, ownerID)

	// Уведомляем пользователя через WS
	if ownerID != "" && h.wsNotifier != nil {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
			defer cancel()
			_ = h.wsNotifier.Notify(bgCtx, ownerID, "task_admin_restored", map[string]string{"task_id": id})
		}()
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// AdminRestoreSharedTask — восстанавливает совместную задачу, удалённую администратором.
func (h *Handler) AdminRestoreSharedTask(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	id := params.ByName("uuid")
	if id == "" {
		http.Error(w, "shared task id required", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	proposerID, addresseeID, err := h.adminSvc.AdminRestoreShared(ctx, id)
	if err != nil {
		h.logger.Errorf("AdminRestoreSharedTask %s: %v", id, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}

	// Синхронно инвалидируем кэш для обоих участников ДО WS-уведомления
	if proposerID != "" {
		_ = h.cache.InvalidateUserLists(ctx, proposerID)
	}
	if addresseeID != "" && addresseeID != proposerID {
		_ = h.cache.InvalidateUserLists(ctx, addresseeID)
	}

	// Уведомляем участников через WS
	if h.wsNotifier != nil {
		payload := map[string]string{"shared_task_id": id}
		if proposerID != "" {
			go func(uid string) {
				bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
				defer cancel()
				_ = h.wsNotifier.Notify(bgCtx, uid, "shared_task_admin_restored", payload)
			}(proposerID)
		}
		if addresseeID != "" && addresseeID != proposerID {
			go func(uid string) {
				bgCtx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
				defer cancel()
				_ = h.wsNotifier.Notify(bgCtx, uid, "shared_task_admin_restored", payload)
			}(addresseeID)
		}
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// AdminGetAllTasks возвращает все задачи всех пользователей. Только для роли admin.
// Поддерживает фильтры: from, to (ISO даты), status, priory.
func (h *Handler) AdminGetAllTasks(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-User-Role") != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	q := r.URL.Query()
	from := q.Get("from")
	to := q.Get("to")
	status := q.Get("status")
	priory := q.Get("priory")

	var tasks []model.TaskList
	var err error

	if from != "" || to != "" || status != "" || priory != "" {
		tasks, err = h.query.AdminFindAllFiltered(ctx, from, to, status, priory)
	} else {
		tasks, err = h.query.AdminFindAllTasks(ctx)
	}

	if err != nil {
		h.logger.Errorf("AdminGetAllTasks: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return nil
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(tasks)
}
