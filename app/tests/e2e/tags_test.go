//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTag_Create(t *testing.T) {
	body := map[string]string{"name": "E2E тег"}
	path := fmt.Sprintf("/v1/users/%s/tags", testUserID)
	req := signedReq(t, http.MethodPost, path, testUserID, body)
	resp := do(t, req)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var tag map[string]interface{}
	decodeJSON(t, resp, &tag)
	assert.Equal(t, "E2E тег", tag["name"])
	assert.NotEmpty(t, tag["id"])
}

func TestTag_GetList(t *testing.T) {
	// Создаём тег
	body := map[string]string{"name": "Тег для списка"}
	req := signedReq(t, http.MethodPost, fmt.Sprintf("/v1/users/%s/tags", testUserID), testUserID, body)
	resp := do(t, req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Получаем список
	req = signedReq(t, http.MethodGet, fmt.Sprintf("/v1/users/%s/tags", testUserID), testUserID, nil)
	resp = do(t, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var tags []interface{}
	decodeJSON(t, resp, &tags)
	assert.NotEmpty(t, tags)
}

func TestTag_GetByID(t *testing.T) {
	// Создаём тег
	body := map[string]string{"name": "Тег для GetByID"}
	req := signedReq(t, http.MethodPost, fmt.Sprintf("/v1/users/%s/tags", testUserID), testUserID, body)
	resp := do(t, req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created map[string]interface{}
	decodeJSON(t, resp, &created)
	tagID := created["id"].(string)

	// Получаем по ID
	req = signedReq(t, http.MethodGet, fmt.Sprintf("/v1/users/%s/tags/%s", testUserID, tagID), testUserID, nil)
	resp = do(t, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var tag map[string]interface{}
	decodeJSON(t, resp, &tag)
	assert.Equal(t, tagID, tag["id"])
}

func TestTag_Update(t *testing.T) {
	// Создаём тег
	body := map[string]string{"name": "Старое имя тега"}
	req := signedReq(t, http.MethodPost, fmt.Sprintf("/v1/users/%s/tags", testUserID), testUserID, body)
	resp := do(t, req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created map[string]interface{}
	decodeJSON(t, resp, &created)
	tagID := created["id"].(string)

	// Обновляем
	req = signedReq(t, http.MethodPatch, fmt.Sprintf("/v1/users/%s/tags/%s", testUserID, tagID), testUserID,
		map[string]string{"name": "Новое имя тега"})
	resp = do(t, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var updated map[string]interface{}
	decodeJSON(t, resp, &updated)
	assert.Equal(t, "Новое имя тега", updated["name"])
}

func TestTag_Delete(t *testing.T) {
	// Создаём тег
	body := map[string]string{"name": "Удаляемый тег"}
	req := signedReq(t, http.MethodPost, fmt.Sprintf("/v1/users/%s/tags", testUserID), testUserID, body)
	resp := do(t, req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created map[string]interface{}
	decodeJSON(t, resp, &created)
	tagID := created["id"].(string)

	// Удаляем
	req = signedReq(t, http.MethodDelete, fmt.Sprintf("/v1/users/%s/tags/%s", testUserID, tagID), testUserID, nil)
	resp = do(t, req)
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent)
}
