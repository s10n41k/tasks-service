package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

var (
	createdTagIDOne string
	createdTagIDTwo string
	baseURL         = "http://localhost:8000"
	userUUID        = "f33fd07e-86b5-44c8-a591-cf52492501f1"
)

func TestCRUDOperations(t *testing.T) {
	// Регистрируем гарантированный cleanup
	t.Cleanup(func() {
		// Silent cleanup (без t.Run, но с логированием ошибок)
		if createdTagIDOne != "" {
			if err := silentDeleteTag(createdTagIDOne); err != nil {
				t.Logf("Cleanup warning: failed to delete tag 1: %v", err)
			}
		}
		if createdTagIDTwo != "" {
			if err := silentDeleteTag(createdTagIDTwo); err != nil {
				t.Logf("Cleanup warning: failed to delete tag 2: %v", err)
			}
		}
	})

	// Основные тесты
	t.Run("CreateTag1", testCreateTagOne)
	t.Run("CreateTag2", testCreateTagTwo)
	t.Run("GetTag", testGetTag)
	t.Run("GetAllTags", testGetAllTags)
	t.Run("UpdateTag", testUpdateTag)

	// Дополнительный явный cleanup для отчётов
	t.Run("CleanupTags", func(t *testing.T) {
		if createdTagIDOne != "" {
			t.Run("DeleteTag1", testDeleteTagOne)
		}
		if createdTagIDTwo != "" {
			t.Run("DeleteTag2", testDeleteTagTwo)
		}
	})
}

func silentDeleteTag(tagID string) error {
	resp, err := http.DefaultClient.Do(newDeleteTagRequest(tagID))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

func newDeleteTagRequest(tagID string) *http.Request {
	req, _ := http.NewRequest("DELETE", baseURL+"/v1/users/"+userUUID+"/tags/"+tagID, nil)
	return req
}

func testCreateTagOne(t *testing.T) {
	url := baseURL + "/v1/users/" + userUUID + "/tags"

	requestBody := map[string]interface{}{
		"name":    "ТЕСТТЕГ",
		"user_id": userUUID,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("Ошибка при маршализации тела запроса: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Ошибка при создании запроса: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Ошибка при создании тега: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("Ожидался статус 201, получен %d. Ответ: %s", resp.StatusCode, string(bodyBytes))
		return
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Ошибка при чтении тела ответа: %v", err)
	}

	responseString := string(bodyBytes)

	var tagID string
	fmt.Sscanf(responseString, "Tag created: %s", &tagID)

	if tagID == "" {
		t.Fatal("ID тега не получен в ответе")
	}
	t.Logf("Создан тег с ID: %s", tagID)

	createdTagIDOne = tagID
}

func testCreateTagTwo(t *testing.T) {
	url := baseURL + "/v1/users/" + userUUID + "/tags"

	requestBody := map[string]interface{}{
		"name":    "ТЕСТТЕГ2",
		"user_id": userUUID,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("Ошибка при маршализации тела запроса: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Ошибка при создании запроса: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Ошибка при создании тега: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("Ожидался статус 201, получен %d. Ответ: %s", resp.StatusCode, string(bodyBytes))
		return
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Ошибка при чтении тела ответа: %v", err)
	}

	responseString := string(bodyBytes)

	var tagID string
	fmt.Sscanf(responseString, "Tag created: %s", &tagID)

	if tagID == "" {
		t.Fatal("ID тега не получен в ответе")
	}
	t.Logf("Создан тег с ID: %s", tagID)

	createdTagIDTwo = tagID
}

func testGetTag(t *testing.T) {
	if createdTagIDOne == "" {
		t.Fatal("ID тега не доступен")
	}

	url := baseURL + "/v1/users/" + userUUID + "/tags/" + createdTagIDOne
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("Ошибка при создании запроса: %v", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Ошибка при получении тега: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("Ожидался статус 200, получен %d. Ответ: %s", resp.StatusCode, string(bodyBytes))
	}

	var tag struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tag); err != nil {
		t.Fatalf("Ошибка парсинга ответа: %v", err)
	}

	if tag.Name != "ТЕСТТЕГ" {
		t.Errorf("Ожидалось имя 'ТЕСТТЕГ', получено '%s'", tag.Name)
	}
}

func testGetAllTags(t *testing.T) {
	url := baseURL + "/v1/users/" + userUUID + "/tags"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("Ошибка при создании запроса: %v", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Ошибка при получении тегов: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("Ожидался статус 200, получен %d. Ответ: %s", resp.StatusCode, string(bodyBytes))
	}

	var tags []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		t.Fatalf("Ошибка парсинга ответа: %v", err)
	}

	if len(tags) < 2 {
		t.Errorf("Ожидалось не менее 2 тегов, получено %d", len(tags))
	}
}

func testUpdateTag(t *testing.T) {
	if createdTagIDOne == "" {
		t.Fatal("ID тега не доступен")
	}

	url := baseURL + "/v1/users/" + userUUID + "/tags/" + createdTagIDOne

	requestBody := map[string]interface{}{
		"name": "ОБНОВЛЕННЫЙ ТЕГ",
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("Ошибка при маршализации тела запроса: %v", err)
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Ошибка при создании запроса: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Ошибка при обновлении тега: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("Ожидался статус 200, получен %d. Ответ: %s", resp.StatusCode, string(bodyBytes))
	}
}

func testDeleteTagOne(t *testing.T) {
	if createdTagIDOne == "" {
		t.Fatal("ID тега не доступен")
	}

	req := newDeleteTagRequest(createdTagIDOne)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Ошибка при удалении тега: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", resp.StatusCode)
	}

	createdTagIDOne = ""
}

func testDeleteTagTwo(t *testing.T) {
	if createdTagIDTwo == "" {
		t.Fatal("ID тега не доступен")
	}

	req := newDeleteTagRequest(createdTagIDTwo)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Ошибка при удалении тега: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", resp.StatusCode)
	}

	createdTagIDTwo = ""
}
