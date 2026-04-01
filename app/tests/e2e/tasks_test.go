//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dueDateStr возвращает дату через 7 дней в формате "YYYY-MM-DD HH:MM".
func dueDateStr() string {
	return time.Now().Add(7 * 24 * time.Hour).Format("2006-01-02 15:04")
}

func TestTask_Create(t *testing.T) {
	body := map[string]interface{}{
		"title":       "E2E Задача",
		"description": "Тест создания задачи",
		"priory":      "green",
		"status":      "not_completed",
		"due_date":    dueDateStr(),
	}
	req := signedReq(t, http.MethodPost, fmt.Sprintf("/v1/users/%s/tasks", testUserID), testUserID, body)
	resp := do(t, req)
	// Задачи без subtasks идут через batch (202) или sync (201)
	assert.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted)
	body2 := readBody(resp)
	assert.True(t, strings.Contains(body2, "Task"), "response should mention Task")
}

func TestTask_CreateWithSubtasks(t *testing.T) {
	body := map[string]interface{}{
		"title":    "Задача с подзадачами",
		"priory":   "red",
		"status":   "not_completed",
		"due_date": dueDateStr(),
		"subtasks": []map[string]string{
			{"title": "Подзадача 1"},
		},
	}
	req := signedReq(t, http.MethodPost, fmt.Sprintf("/v1/users/%s/tasks", testUserID), testUserID, body)
	resp := do(t, req)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	b := readBody(resp)
	assert.Contains(t, b, "Task created with ID:")
}

func TestTask_GetUserTasks(t *testing.T) {
	// Сначала создаём задачу (sync path через subtasks)
	createBody := map[string]interface{}{
		"title":    "Задача для списка",
		"priory":   "blue",
		"due_date": dueDateStr(),
		"subtasks": []map[string]string{{"title": "st"}},
	}
	req := signedReq(t, http.MethodPost, fmt.Sprintf("/v1/users/%s/tasks", testUserID), testUserID, createBody)
	resp := do(t, req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Получаем список задач пользователя
	req = signedReq(t, http.MethodGet, fmt.Sprintf("/v1/users/%s/tasks", testUserID), testUserID, nil)
	resp = do(t, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result interface{}
	decodeJSON(t, resp, &result)
}

func TestTask_GetByID(t *testing.T) {
	// Создаём задачу и получаем ID
	createBody := map[string]interface{}{
		"title":    "Задача для GetByID",
		"priory":   "blue",
		"due_date": dueDateStr(),
		"subtasks": []map[string]string{{"title": "sub"}},
	}
	req := signedReq(t, http.MethodPost, fmt.Sprintf("/v1/users/%s/tasks", testUserID), testUserID, createBody)
	resp := do(t, req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	b := readBody(resp)
	// Ответ: "Task created with ID: <uuid>"
	parts := strings.Split(b, ": ")
	require.Len(t, parts, 2)
	taskID := strings.TrimSpace(parts[1])

	// Получаем задачу по ID
	req = signedReq(t, http.MethodGet, "/tasks/"+taskID, testUserID, nil)
	resp = do(t, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var task map[string]interface{}
	decodeJSON(t, resp, &task)
	assert.Equal(t, taskID, task["id"])
}

func TestTask_Update(t *testing.T) {
	// Создаём задачу
	createBody := map[string]interface{}{
		"title":    "Задача для Update",
		"priory":   "blue",
		"due_date": dueDateStr(),
		"subtasks": []map[string]string{{"title": "sub"}},
	}
	req := signedReq(t, http.MethodPost, fmt.Sprintf("/v1/users/%s/tasks", testUserID), testUserID, createBody)
	resp := do(t, req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	b := readBody(resp)
	taskID := strings.TrimSpace(strings.Split(b, ": ")[1])

	// Обновляем
	updateBody := map[string]interface{}{"title": "Обновлённая задача"}
	req = signedReq(t, http.MethodPatch, "/tasks/"+taskID, testUserID, updateBody)
	resp = do(t, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var task map[string]interface{}
	decodeJSON(t, resp, &task)
	assert.Equal(t, "Обновлённая задача", task["title"])
}

func TestTask_Delete(t *testing.T) {
	// Создаём задачу
	createBody := map[string]interface{}{
		"title":    "Задача для Delete",
		"priory":   "blue",
		"due_date": dueDateStr(),
		"subtasks": []map[string]string{{"title": "sub"}},
	}
	req := signedReq(t, http.MethodPost, fmt.Sprintf("/v1/users/%s/tasks", testUserID), testUserID, createBody)
	resp := do(t, req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	b := readBody(resp)
	taskID := strings.TrimSpace(strings.Split(b, ": ")[1])

	// Удаляем
	req = signedReq(t, http.MethodDelete, "/tasks/"+taskID, testUserID, nil)
	resp = do(t, req)
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent)

	// Проверяем что задача больше не находится
	req = signedReq(t, http.MethodGet, "/tasks/"+taskID, testUserID, nil)
	resp = do(t, req)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestTask_Unauthorized(t *testing.T) {
	// Запрос без заголовков подписи → 401
	resp, err := http.Get(url(fmt.Sprintf("/v1/users/%s/tasks", testUserID)))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// --- Subtasks ---

func createTaskAndGetID(t *testing.T) string {
	t.Helper()
	body := map[string]interface{}{
		"title":    "Задача для подзадач",
		"priory":   "blue",
		"due_date": dueDateStr(),
		"subtasks": []map[string]string{{"title": "initial sub"}},
	}
	req := signedReq(t, http.MethodPost, fmt.Sprintf("/v1/users/%s/tasks", testUserID), testUserID, body)
	resp := do(t, req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	b := readBody(resp)
	taskID := strings.TrimSpace(strings.Split(b, ": ")[1])
	return taskID
}

func TestSubtask_Add(t *testing.T) {
	taskID := createTaskAndGetID(t)

	body := map[string]string{"title": "Новая подзадача"}
	path := fmt.Sprintf("/v1/users/%s/tasks/%s/subtasks", testUserID, taskID)
	req := signedReq(t, http.MethodPost, path, testUserID, body)
	resp := do(t, req)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var sub map[string]interface{}
	decodeJSON(t, resp, &sub)
	assert.Equal(t, "Новая подзадача", sub["title"])
}

func TestSubtask_Toggle(t *testing.T) {
	taskID := createTaskAndGetID(t)

	// Добавляем подзадачу
	body := map[string]string{"title": "Toggle подзадача"}
	path := fmt.Sprintf("/v1/users/%s/tasks/%s/subtasks", testUserID, taskID)
	req := signedReq(t, http.MethodPost, path, testUserID, body)
	resp := do(t, req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var sub map[string]interface{}
	decodeJSON(t, resp, &sub)
	subID := sub["id"].(string)

	// Переключаем состояние
	togglePath := fmt.Sprintf("/v1/users/%s/tasks/%s/subtasks/%s", testUserID, taskID, subID)
	req = signedReq(t, http.MethodPatch, togglePath, testUserID, nil)
	resp = do(t, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestSubtask_Update(t *testing.T) {
	taskID := createTaskAndGetID(t)

	// Добавляем подзадачу
	body := map[string]string{"title": "Старый заголовок"}
	path := fmt.Sprintf("/v1/users/%s/tasks/%s/subtasks", testUserID, taskID)
	req := signedReq(t, http.MethodPost, path, testUserID, body)
	resp := do(t, req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var sub map[string]interface{}
	decodeJSON(t, resp, &sub)
	subID := sub["id"].(string)

	// Обновляем заголовок
	updatePath := fmt.Sprintf("/v1/users/%s/tasks/%s/subtasks/%s", testUserID, taskID, subID)
	req = signedReq(t, http.MethodPut, updatePath, testUserID, map[string]string{"title": "Новый заголовок"})
	resp = do(t, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestSubtask_Delete(t *testing.T) {
	taskID := createTaskAndGetID(t)

	// Добавляем подзадачу
	body := map[string]string{"title": "Удаляемая подзадача"}
	path := fmt.Sprintf("/v1/users/%s/tasks/%s/subtasks", testUserID, taskID)
	req := signedReq(t, http.MethodPost, path, testUserID, body)
	resp := do(t, req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var sub map[string]interface{}
	decodeJSON(t, resp, &sub)
	subID := sub["id"].(string)

	// Удаляем
	deletePath := fmt.Sprintf("/v1/users/%s/tasks/%s/subtasks/%s", testUserID, taskID, subID)
	req = signedReq(t, http.MethodDelete, deletePath, testUserID, nil)
	resp = do(t, req)
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent)
}
