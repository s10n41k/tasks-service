//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proposeSharedTask создаёт совместную задачу от testUserID к testUserID2 и возвращает её ID.
func proposeSharedTask(t *testing.T) string {
	t.Helper()
	body := map[string]interface{}{
		"addressee_id": testUserID2,
		"title":        "Совместная задача E2E",
		"description":  "Описание",
		"priority":     "red",
		"due_date":     dueDateStr(),
		"subtasks": []map[string]string{
			{"title": "Моя подзадача", "assignee_id": testUserID},
			{"title": "Твоя подзадача", "assignee_id": testUserID2},
		},
	}
	path := fmt.Sprintf("/v1/users/%s/shared-tasks", testUserID)
	req := signedReq(t, http.MethodPost, path, testUserID, body)
	resp := do(t, req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var result map[string]string
	decodeJSON(t, resp, &result)
	id := result["id"]
	require.NotEmpty(t, id)
	return id
}

func TestSharedTask_Propose(t *testing.T) {
	id := proposeSharedTask(t)
	assert.NotEmpty(t, id)
}

func TestSharedTask_GetList(t *testing.T) {
	proposeSharedTask(t)

	// Получаем список для proposer
	req := signedReq(t, http.MethodGet, fmt.Sprintf("/v1/users/%s/shared-tasks", testUserID), testUserID, nil)
	resp := do(t, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var tasks []interface{}
	decodeJSON(t, resp, &tasks)
	assert.NotEmpty(t, tasks)
}

func TestSharedTask_GetByID(t *testing.T) {
	id := proposeSharedTask(t)

	req := signedReq(t, http.MethodGet, fmt.Sprintf("/v1/users/%s/shared-tasks/%s", testUserID, id), testUserID, nil)
	resp := do(t, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var task map[string]interface{}
	decodeJSON(t, resp, &task)
	assert.Equal(t, id, task["id"])
	assert.Equal(t, "pending", task["status"])
}

func TestSharedTask_Accept(t *testing.T) {
	id := proposeSharedTask(t)

	// Адресат принимает задачу
	path := fmt.Sprintf("/v1/users/%s/shared-tasks/%s/accept", testUserID2, id)
	req := signedReq(t, http.MethodPost, path, testUserID2, nil)
	resp := do(t, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Проверяем статус
	req = signedReq(t, http.MethodGet, fmt.Sprintf("/v1/users/%s/shared-tasks/%s", testUserID, id), testUserID, nil)
	resp = do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var task map[string]interface{}
	decodeJSON(t, resp, &task)
	assert.Equal(t, "accepted", task["status"])
}

func TestSharedTask_Reject(t *testing.T) {
	id := proposeSharedTask(t)

	// Адресат отклоняет задачу
	path := fmt.Sprintf("/v1/users/%s/shared-tasks/%s/reject", testUserID2, id)
	req := signedReq(t, http.MethodPost, path, testUserID2, nil)
	resp := do(t, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Проверяем статус
	req = signedReq(t, http.MethodGet, fmt.Sprintf("/v1/users/%s/shared-tasks/%s", testUserID, id), testUserID, nil)
	resp = do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var task map[string]interface{}
	decodeJSON(t, resp, &task)
	assert.Equal(t, "rejected", task["status"])
}

func TestSharedTask_ToggleSubtask(t *testing.T) {
	id := proposeSharedTask(t)

	// Адресат принимает задачу
	path := fmt.Sprintf("/v1/users/%s/shared-tasks/%s/accept", testUserID2, id)
	req := signedReq(t, http.MethodPost, path, testUserID2, nil)
	resp := do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Получаем детали задачи для ID подзадачи, назначенной testUserID
	detailPath := fmt.Sprintf("/v1/users/%s/shared-tasks/%s", testUserID, id)
	req = signedReq(t, http.MethodGet, detailPath, testUserID, nil)
	resp = do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var task map[string]interface{}
	decodeJSON(t, resp, &task)

	subtasks, ok := task["subtasks"].([]interface{})
	require.True(t, ok)
	require.NotEmpty(t, subtasks)

	// Ищем подзадачу назначенную testUserID
	var subID string
	for _, s := range subtasks {
		sub := s.(map[string]interface{})
		if sub["assignee_id"] == testUserID {
			subID = sub["id"].(string)
			break
		}
	}
	require.NotEmpty(t, subID, "subtask for testUserID not found")

	// Переключаем состояние подзадачи
	togglePath := fmt.Sprintf("/v1/users/%s/shared-tasks/%s/subtasks/%s", testUserID, id, subID)
	req = signedReq(t, http.MethodPatch, togglePath, testUserID, nil)
	resp = do(t, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestSharedTask_ProposeWithoutSubscription_Forbidden(t *testing.T) {
	// Используем пользователя без подписки (случайный UUID)
	noSubUser := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	body := map[string]interface{}{
		"addressee_id": testUserID2,
		"title":        "Не должна создаться",
		"subtasks": []map[string]string{
			{"title": "sub1", "assignee_id": noSubUser},
			{"title": "sub2", "assignee_id": testUserID2},
		},
	}
	path := fmt.Sprintf("/v1/users/%s/shared-tasks", noSubUser)
	req := signedReq(t, http.MethodPost, path, noSubUser, body)
	resp := do(t, req)
	assert.Equal(t, http.StatusPaymentRequired, resp.StatusCode)
}
