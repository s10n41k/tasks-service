package handlers

import (
	"TODOLIST_Tasks/app/internal/apperror"
	"TODOLIST_Tasks/app/internal/shared_tasks/domain"
	sharedDTO "TODOLIST_Tasks/app/internal/shared_tasks/dto"
	"TODOLIST_Tasks/app/internal/shared_tasks/port"
	"TODOLIST_Tasks/app/internal/shared_tasks/service"
	"TODOLIST_Tasks/app/internal/tasks/notification"
	tasksSvc "TODOLIST_Tasks/app/internal/tasks/service"
	"TODOLIST_Tasks/app/pkg/api/signature"
	logging "TODOLIST_Tasks/app/pkg/logging"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
)

const (
	sharedTasksURL       = "/v1/users/:userId/shared-tasks"
	sharedTaskDetailURL  = "/v1/users/:userId/shared-tasks/:stid"
	sharedTaskAcceptURL  = "/v1/users/:userId/shared-tasks/:stid/accept"
	sharedTaskRejectURL  = "/v1/users/:userId/shared-tasks/:stid/reject"
	sharedSubtaskDoneURL = "/v1/users/:userId/shared-tasks/:stid/subtasks/:sid"

	adminSharedTasksURL = "/admin/shared-tasks"
)

// Handler — HTTP-обработчик для домена SharedTask.
type Handler struct {
	svc      service.SharedTaskService
	subSvc   tasksSvc.SubscriptionService
	usersMsg *notification.UsersMessageClient
	logger   *logging.Logger
}

func New(svc service.SharedTaskService, usersMsg *notification.UsersMessageClient, subSvc tasksSvc.SubscriptionService) *Handler {
	return &Handler{
		svc:      svc,
		subSvc:   subSvc,
		usersMsg: usersMsg,
		logger:   logging.GetLogger().GetLoggerWithField("handler", "shared_tasks"),
	}
}

func (h *Handler) Register(router *httprouter.Router) {
	router.HandlerFunc(http.MethodGet, sharedTasksURL, signature.Middleware(apperror.Middleware(h.GetList)))
	router.HandlerFunc(http.MethodPost, sharedTasksURL, signature.Middleware(apperror.Middleware(h.Propose)))
	router.HandlerFunc(http.MethodGet, sharedTaskDetailURL, signature.Middleware(apperror.Middleware(h.GetByID)))
	router.HandlerFunc(http.MethodPatch, sharedTaskDetailURL, signature.Middleware(apperror.Middleware(h.Update)))
	router.HandlerFunc(http.MethodDelete, sharedTaskDetailURL, signature.Middleware(apperror.Middleware(h.Delete)))
	router.HandlerFunc(http.MethodPost, sharedTaskAcceptURL, signature.Middleware(apperror.Middleware(h.Accept)))
	router.HandlerFunc(http.MethodPost, sharedTaskRejectURL, signature.Middleware(apperror.Middleware(h.Reject)))
	router.HandlerFunc(http.MethodPatch, sharedSubtaskDoneURL, signature.Middleware(apperror.Middleware(h.ToggleSubtaskDone)))

	// Admin endpoint
	router.HandlerFunc(http.MethodGet, adminSharedTasksURL, signature.Middleware(apperror.Middleware(h.AdminGetAllSharedTasks)))
}

func (h *Handler) GetList(w http.ResponseWriter, r *http.Request) error {
	userID := resolveUserID(r)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	tasks, err := h.svc.FindByUser(ctx, userID)
	if err != nil {
		h.logger.Errorf("GetList user %s: %v", userID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}
	if tasks == nil {
		tasks = []domain.SharedTask{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(sharedDTO.ToResponses(tasks))
}

func (h *Handler) Propose(w http.ResponseWriter, r *http.Request) error {
	userID := resolveUserID(r)

	if !h.hasActiveSubscription(r, userID) {
		http.Error(w, "subscription required", http.StatusPaymentRequired)
		return nil
	}

	var body struct {
		AddresseeID string             `json:"addressee_id"`
		Title       string             `json:"title"`
		Description string             `json:"description"`
		Priority    string             `json:"priority"`
		DueDate     string             `json:"due_date"`
		Subtasks    []port.SubtaskInput `json:"subtasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return nil
	}
	if body.Title == "" || body.AddresseeID == "" {
		http.Error(w, "title и addressee_id обязательны", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	id, err := h.svc.Propose(ctx, userID, body.AddresseeID, body.Title, body.Description, body.Priority, body.DueDate, body.Subtasks)
	if err != nil {
		h.logger.Errorf("Propose user %s: %v", userID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	return json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	taskID := params.ByName("stid")
	userID := resolveUserID(r)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	task, err := h.svc.FindByID(ctx, taskID, userID)
	if err != nil {
		h.logger.Errorf("GetByID task %s user %s: %v", taskID, userID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}
	if task == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(sharedDTO.ToResponse(*task))
}

func (h *Handler) Accept(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	taskID := params.ByName("stid")
	userID := resolveUserID(r)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.svc.Accept(ctx, taskID, userID); err != nil {
		h.logger.Errorf("Accept task %s user %s: %v", taskID, userID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	taskID := params.ByName("stid")
	userID := resolveUserID(r)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.svc.Reject(ctx, taskID, userID); err != nil {
		h.logger.Errorf("Reject task %s user %s: %v", taskID, userID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	taskID := params.ByName("stid")
	userID := resolveUserID(r)

	var body struct {
		Title       string              `json:"title"`
		Description string              `json:"description"`
		Priority    string              `json:"priority"`
		DueDate     string              `json:"due_date"`
		Subtasks    *[]port.SubtaskInput `json:"subtasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		http.Error(w, "title обязателен", http.StatusBadRequest)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Читаем задачу до изменения — для авто-сообщения партнёру
	oldTask, _ := h.svc.FindByID(ctx, taskID, userID)

	input := port.UpdateInput{
		Title:       body.Title,
		Description: body.Description,
		Priority:    body.Priority,
		DueDate:     body.DueDate,
		Subtasks:    body.Subtasks,
	}
	wasCounter, err := h.svc.Update(ctx, taskID, userID, input)
	if err != nil {
		h.logger.Errorf("Update task %s user %s: %v", taskID, userID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}

	if oldTask != nil && h.usersMsg != nil {
		partnerID := oldTask.ProposerID
		if userID == oldTask.ProposerID {
			partnerID = oldTask.AddresseeID
		}
		if wasCounter {
			go h.sendCounterProposeMessage(userID, partnerID, body.Title, oldTask.Title)
		} else {
			go h.sendUpdateMessage(userID, partnerID, oldTask.Title, oldTask.Priority, oldTask.DueDate, input)
		}
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	taskID := params.ByName("stid")
	userID := resolveUserID(r)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	partnerID, taskTitle, err := h.svc.Delete(ctx, taskID, userID)
	if err != nil {
		h.logger.Errorf("Delete task %s user %s: %v", taskID, userID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}

	if partnerID != "" && h.usersMsg != nil {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			content := fmt.Sprintf("🗑️ Участник удалил нашу совместную задачу «%s».", taskTitle)
			if err := h.usersMsg.SendMessage(bgCtx, userID, partnerID, content); err != nil {
				h.logger.Warnf("Delete auto-message: %v", err)
			}
		}()
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) ToggleSubtaskDone(w http.ResponseWriter, r *http.Request) error {
	params := httprouter.ParamsFromContext(r.Context())
	subtaskID := params.ByName("sid")
	userID := resolveUserID(r)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	subtask, err := h.svc.ToggleSubtaskDone(ctx, subtaskID, userID)
	if err != nil {
		h.logger.Errorf("ToggleSubtaskDone subtask %s user %s: %v", subtaskID, userID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(map[string]bool{"is_done": subtask.IsDone})
}

// --- helpers ---

// hasActiveSubscription проверяет наличие активной подписки двумя способами:
// 1. Быстрый путь: JWT-заголовок X-User-Subscription == "pro" (актуально после рефреша токена).
// 2. Fallback: запрос в локальную таблицу user_subscriptions через SubscriptionService.
//    Нужен потому что JWT может содержать устаревшее значение subscription=false
//    если пользователь купил подписку не перелогинившись (токен ещё не обновлён).
func (h *Handler) hasActiveSubscription(r *http.Request, userID string) bool {
	if r.Header.Get("X-User-Subscription") == "pro" {
		return true
	}
	if h.subSvc == nil || userID == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	return h.subSvc.IsActive(ctx, userID)
}

func resolveUserID(r *http.Request) string {
	if uid := r.Header.Get("X-User-ID"); uid != "" {
		return uid
	}
	params := httprouter.ParamsFromContext(r.Context())
	return params.ByName("userId")
}

// sendCounterProposeMessage — уведомляет партнёра о встречном предложении.
// Вызывается всегда, даже если изменены только подзадачи.
func (h *Handler) sendCounterProposeMessage(userID, partnerID, newTitle, oldTitle string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	title := newTitle
	if title == "" {
		title = oldTitle
	}
	content := fmt.Sprintf(
		"↩️ Партнёр внёс изменения в задачу «%s» и предлагает её на ваше рассмотрение. Откройте задачу и примите или отклоните предложение.",
		title,
	)
	if err := h.usersMsg.SendMessage(ctx, userID, partnerID, content); err != nil {
		h.logger.Warnf("CounterPropose auto-message: %v", err)
	}
}

func (h *Handler) sendUpdateMessage(userID, partnerID, oldTitle, oldPriority string, oldDueDate *time.Time, input port.UpdateInput) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	prioLabel := map[string]string{"red": "🔴 Красный", "blue": "🔵 Синий", "green": "🟢 Зелёный"}
	var changes []string

	if input.Title != "" && input.Title != oldTitle {
		changes = append(changes, fmt.Sprintf("📝 Название: «%s» → «%s»", oldTitle, input.Title))
	}
	if input.Priority != "" && input.Priority != oldPriority {
		old := prioLabel[oldPriority]
		newP := prioLabel[input.Priority]
		if old == "" {
			old = oldPriority
		}
		if newP == "" {
			newP = input.Priority
		}
		changes = append(changes, fmt.Sprintf("🎯 Приоритет: %s → %s", old, newP))
	}
	if input.DueDate == "" && oldDueDate != nil {
		changes = append(changes, "📅 Дедлайн убран")
	} else if input.DueDate != "" {
		t, err := time.Parse(time.RFC3339, input.DueDate)
		if err == nil {
			changes = append(changes, fmt.Sprintf("📅 Дедлайн: %s", t.Format("02.01.2006 15:04")))
		}
	}

	if len(changes) == 0 {
		return
	}

	title := input.Title
	if title == "" {
		title = oldTitle
	}
	content := fmt.Sprintf("✏️ Участник изменил нашу задачу «%s»:\n%s", title, strings.Join(changes, "\n"))
	if err := h.usersMsg.SendMessage(ctx, userID, partnerID, content); err != nil {
		h.logger.Warnf("Update auto-message: %v", err)
	}
}

// AdminGetAllSharedTasks возвращает все совместные задачи всех пользователей. Только для роли admin.
func (h *Handler) AdminGetAllSharedTasks(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("X-User-Role") != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tasks, err := h.svc.AdminFindAll(ctx)
	if err != nil {
		h.logger.Errorf("AdminGetAllSharedTasks: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return nil
	}
	if tasks == nil {
		tasks = []domain.SharedTask{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(sharedDTO.ToResponses(tasks))
}
