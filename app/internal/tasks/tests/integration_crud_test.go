package tests

import (
	"TODOLIST_Tasks/app/internal/tasks/model"
	"TODOLIST_Tasks/app/pkg/api/resilience"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	redis2 "github.com/go-redis/redis/v8"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	createdTaskIDOne string
	createdTaskIDTwo string
	baseURL          = "http://localhost:8000"
	userUUID         = "f33fd07e-86b5-44c8-a591-cf52492501f1"
)

func TestCRUDOperations(t *testing.T) {
	// Регистрируем гарантированный cleanup
	t.Cleanup(func() {
		// Silent cleanup (без t.Run, но с логированием ошибок)
		if createdTaskIDOne != "" {
			if err := silentDeleteTask(createdTaskIDOne); err != nil {
				t.Logf("Cleanup warning: failed to delete task 1: %v", err)
			}
		}
		if createdTaskIDTwo != "" {
			if err := silentDeleteTask(createdTaskIDTwo); err != nil {
				t.Logf("Cleanup warning: failed to delete task 2: %v", err)
			}
		}
	})

	// Основные тесты
	t.Run("CreateTask1", testCreateTaskOne)
	t.Run("CreateTask2", testCreateTaskTwo)
	t.Run("GetTask", testGetTask)
	t.Run("GetAllTask", testGetAllTask)
	t.Run("GetAllTaskByTag", testGetAllTaskByTag)
	t.Run("UpdateTask", testUpdateTask)

	// Дополнительный явный cleanup для отчётов
	t.Run("CleanupTasks", func(t *testing.T) {
		if createdTaskIDOne != "" {
			t.Run("DeleteTask1", testDeleteTaskOne)
		}
		if createdTaskIDTwo != "" {
			t.Run("DeleteTask2", testDeleteTaskTwo)
		}
	})
}

func TestConcurrentCreateTasks(t *testing.T) {
	const (
		concurrentRequests = 1000
		requestTimeout     = 10 * time.Second
	)

	var (
		wg          sync.WaitGroup
		errors      = make(chan error, concurrentRequests)
		successChan = make(chan struct{}, concurrentRequests) // Используем пустую структуру для экономии памяти
		taskIDs     = make([]string, 0, concurrentRequests)
		mutex       sync.Mutex
	)

	startTime := time.Now()

	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func(requestNum int) {
			defer wg.Done()

			// Генерируем уникальные данные
			requestBody := map[string]interface{}{
				"title":       fmt.Sprintf("LoadTest-%d-%d", requestNum, time.Now().UnixNano()),
				"description": fmt.Sprintf("Concurrency test #%d", requestNum),
				"status":      "not_completed",
				"priory":      []string{"green", "blue", "red"}[requestNum%3],
				"due_date":    time.Now().Add(time.Hour * 24).Format("2006-01-02 15:04"),
				"tag_id":      "2f36a1ba-8f13-45dc-8bdf-d70ec2345030",
			}

			body, err := json.Marshal(requestBody)
			if err != nil {
				errors <- fmt.Errorf("request %d: marshal error: %v", requestNum, err)
				return
			}

			req, err := http.NewRequest(
				"POST",
				baseURL+"/v1/users/"+userUUID+"/tasks",
				bytes.NewBuffer(body),
			)
			if err != nil {
				errors <- fmt.Errorf("request %d: create request error: %v", requestNum, err)
				return
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: requestTimeout}
			resp, err := client.Do(req)
			if err != nil {
				errors <- fmt.Errorf("request %d: send request error: %v", requestNum, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusCreated {
				bodyBytes, _ := io.ReadAll(resp.Body)
				errors <- fmt.Errorf("request %d: expected 201, got %d. Response: %s",
					requestNum, resp.StatusCode, string(bodyBytes))
				return
			}

			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				errors <- fmt.Errorf("request %d: read response error: %v", requestNum, err)
				return
			}

			var taskID string
			if _, err := fmt.Sscanf(string(bodyBytes), "Task created with ID: %s", &taskID); err != nil {
				errors <- fmt.Errorf("request %d: parse task ID error: %v", requestNum, err)
				return
			}

			mutex.Lock()
			taskIDs = append(taskIDs, taskID)
			mutex.Unlock()

			successChan <- struct{}{}
		}(i)
	}

	wg.Wait()
	close(successChan)
	close(errors)

	duration := time.Since(startTime)
	successCount := len(successChan)

	t.Logf("\nLoad Test Results:")
	t.Logf("Total requests: %d", concurrentRequests)
	t.Logf("Successful:     %d (%.1f%%)", successCount, float64(successCount)/float64(concurrentRequests)*100)
	t.Logf("Time elapsed:   %v", duration.Round(time.Millisecond))
	t.Logf("RPS:           %.1f", float64(successCount)/duration.Seconds())

	// Вывод первых 3 ошибок (если есть)
	errorCount := 0
	for err := range errors {
		if errorCount < 3 {
			t.Error(err)
		}
		errorCount++
	}
	if errorCount > 3 {
		t.Errorf("... and %d more errors", errorCount-3)
	}

	if successCount != concurrentRequests {
		t.Errorf("Expected %d successful requests, got %d", concurrentRequests, successCount)
	}

	// Параллельная очистка
	t.Cleanup(func() {
		t.Log("Cleaning up test tasks from cache and database...")

		var (
			cleanupWg     sync.WaitGroup
			cleanupErrors = make(chan error, len(taskIDs)*2) // две операции на задачу
			ctx, cancel   = context.WithTimeout(context.Background(), 30*time.Second)
		)
		defer cancel()

		for _, id := range taskIDs {
			cleanupWg.Add(2)

			// Удаление из кеша Redis
			go func(taskID string) {
				defer cleanupWg.Done()
				key := fmt.Sprintf("task:%s", taskID)
				err := redis2.Client{}.Del(ctx, key).Err()
				if err != nil {
					cleanupErrors <- fmt.Errorf("failed to delete task %s from cache: %v", taskID, err)
				}
			}(id)

			// Удаление из PostgreSQL через HTTP API
			go func(taskID string) {
				defer cleanupWg.Done()
				url := fmt.Sprintf("http://localhost:8000/tasks/%s", taskID)
				req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
				if err != nil {
					cleanupErrors <- fmt.Errorf("failed to create delete request for task %s: %v", taskID, err)
					return
				}

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					cleanupErrors <- fmt.Errorf("failed to delete task %s from server: %v", taskID, err)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
					bodyBytes, _ := io.ReadAll(resp.Body)
					cleanupErrors <- fmt.Errorf(
						"unexpected status when deleting task %s: %d, response: %s",
						taskID, resp.StatusCode, string(bodyBytes),
					)
				}
			}(id)
		}

		// Ждём завершения всех горутин или таймаута
		done := make(chan struct{})
		go func() {
			cleanupWg.Wait()
			close(done)
		}()

		select {
		case <-done:
			t.Log("All cleanup goroutines finished")
		case <-ctx.Done():
			t.Error("Cleanup timeout exceeded")
		}

		close(cleanupErrors)
		for err := range cleanupErrors {
			t.Error(err)
		}
	})
}

// Вспомогательная функция для "тихого" удаления
func silentDeleteTask(taskID string) error {
	resp, err := http.DefaultClient.Do(newDeleteRequest(taskID))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

func newDeleteRequest(taskID string) *http.Request {
	req, _ := http.NewRequest("DELETE", baseURL+"/tasks/"+taskID, nil)
	return req
}

func testCreateTaskOne(t *testing.T) {
	url := baseURL + "/v1/users/" + userUUID + "/tasks"

	requestBody := map[string]interface{}{
		"title":       "ТЕСТ",
		"description": "ТЕСТ 2",
		"status":      "not_completed",
		"priory":      "green",
		"due_date":    "2025-03-01 15:40",
		"tag_id":      "1d1417c3-6792-42c1-94cd-77ef98c063d7",
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
		t.Fatalf("Ошибка при создании задачи: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body) // Читаем тело ответа
		t.Errorf("Ожидался статус 201, получен %d. Ответ: %s", resp.StatusCode, bodyBytes)
		return // Выходим из функции после логирования
	}

	bodyBytes, err := io.ReadAll(resp.Body) // Читаем тело ответа
	if err != nil {
		t.Fatalf("Ошибка при чтении тела ответа: %v", err)
	}

	responseString := string(bodyBytes)

	var taskID string
	fmt.Sscanf(responseString, "Task created with ID: %s", &taskID)

	if taskID == "" {
		t.Fatal("ID задачи не получен в ответе")
	}
	t.Logf("Создана задача с ID: %s", taskID)

	createdTaskIDOne = taskID
}

func testCreateTaskTwo(t *testing.T) {
	url := baseURL + "/v1/users/" + userUUID + "/tasks"

	requestBody := map[string]interface{}{
		"title":       "Test228888",
		"description": "ТЕСТ 2 TWO",
		"status":      "not_completed",
		"priory":      "red",
		"due_date":    "2025-03-01 15:40",
		"tag_id":      "2f36a1ba-8f13-45dc-8bdf-d70ec2345030",
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
		t.Fatalf("Ошибка при создании задачи: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body) // Читаем тело ответа
		t.Errorf("Ожидался статус 201, получен %d. Ответ: %s", resp.StatusCode, bodyBytes)
		return // Выходим из функции после логирования
	}

	bodyBytes, err := io.ReadAll(resp.Body) // Читаем тело ответа
	if err != nil {
		t.Fatalf("Ошибка при чтении тела ответа: %v", err)
	}

	responseString := string(bodyBytes)

	var taskID string
	fmt.Sscanf(responseString, "Task created with ID: %s", &taskID)

	if taskID == "" {
		t.Fatal("ID задачи не получен в ответе")
	}
	t.Logf("Создана задача с ID: %s", taskID)

	createdTaskIDTwo = taskID
}

func testGetTask(t *testing.T) {
	if createdTaskIDOne == "" {
		t.Fatal("ID задачи не доступен")
	}

	url := baseURL + "/tasks/" + createdTaskIDOne
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Ошибка при получении задачи: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", resp.StatusCode)
	}

	var task struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		t.Fatalf("Ошибка парсинга ответа: %v", err)
	}

	if task.Title != "ТЕСТ" {
		t.Errorf("Ожидался заголовок 'ТЕСТ', получен '%s'", task.Title)
	}
}

func testGetAllTask(t *testing.T) {
	url := baseURL + "/v1/users/" + userUUID + "/tasks?title=eq:Test228888"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("Ошибка при создании запроса: %v", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Ошибка при получении задач: %v", err)
	}
	if resp == nil {
		t.Fatal("Ответ от сервера равен nil")
	}
	defer resp.Body.Close()

	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", resp.StatusCode)
		return // Добавляем return, чтобы не продолжать выполнение теста
	}

	// Декодируем ответ в массив задач
	var tasks []model.TaskResponse // Используем правильную структуру для декодирования
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		t.Fatalf("Ошибка парсинга ответа: %v", err)
	}

	// Проверяем, что массив tasks не пустой
	if len(tasks) == 0 {
		t.Fatal("Задачи не найдены")
	}

	// Проверяем заголовок первой задачи
	expectedTitle := "Test228888"
	if tasks[0].Title != expectedTitle {
		t.Errorf("Ожидался заголовок '%s', получен '%s'", expectedTitle, tasks[0].Title)
	}
}
func testGetAllTaskByTag(t *testing.T) {

	url := baseURL + "/v1/users/" + userUUID + "/tags/1d1417c3-6792-42c1-94cd-77ef98c063d7/tasks"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("Ошибка при создании запроса: %v", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Ошибка при получении задач: %v", err)
	}
	defer resp.Body.Close()

	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", resp.StatusCode)
		return // Добавляем return, чтобы не продолжать выполнение теста
	}

	// Декодируем ответ в массив задач
	var tasks []model.TaskResponse // Используем правильную структуру для декодирования
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		t.Fatalf("Ошибка парсинга ответа: %v", err)
	}

	// Проверяем, что массив tasks не пустой
	if len(tasks) == 0 {
		t.Fatal("Задачи не найдены")
	}

	// Проверяем заголовок первой задачи (или любое другое поле по вашему выбору)
	expectedTitle := "ТЕСТ" // Замените на ожидаемое значение заголовка
	if tasks[0].Title != expectedTitle {
		t.Errorf("Ожидался заголовок '%s', получен '%s'", expectedTitle, tasks[0].Title)
	}
}

func testUpdateTask(t *testing.T) {
	if createdTaskIDOne == "" {
		t.Fatal("ID задачи не доступен")
	}

	url := baseURL + "/tasks/" + createdTaskIDOne
	requestBody := map[string]string{
		"title": "ТЕСТ ОБНОВЛЕННЫЙ",
	}

	body, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("PATCH", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Ошибка при обновлении задачи: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", resp.StatusCode)
	}
}

func testDeleteTaskOne(t *testing.T) {
	if createdTaskIDOne == "" {
		t.Fatal("ID задачи не доступен")
	}

	url := baseURL + "/tasks/" + createdTaskIDOne
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Ошибка при удалении задачи: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", resp.StatusCode)
	}
}

func testDeleteTaskTwo(t *testing.T) {
	if createdTaskIDTwo == "" {
		t.Fatal("ID задачи не доступен")
	}

	url := baseURL + "/tasks/" + createdTaskIDTwo
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Ошибка при удалении задачи: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", resp.StatusCode)
	}

}

func testCreateTask(t *testing.T) {
	url := baseURL + "/v1/users/" + userUUID + "/tasks"

	requestBody := map[string]interface{}{
		"title":       "ТЕСТ",
		"description": "ТЕСТ 2",
		"status":      "not_completed",
		"priory":      "green",
		"due_date":    "2025-03-01 15:40",
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
		t.Fatalf("Ошибка при создании задачи: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body) // Читаем тело ответа
		t.Errorf("Ожидался статус 201, получен %d. Ответ: %s", resp.StatusCode, bodyBytes)
		return // Выходим из функции после логирования
	}

	bodyBytes, err := io.ReadAll(resp.Body) // Читаем тело ответа
	if err != nil {
		t.Fatalf("Ошибка при чтении тела ответа: %v", err)
	}

	responseString := string(bodyBytes)

	var taskID string
	fmt.Sscanf(responseString, "Task created with ID: %s", &taskID)

	if taskID == "" {
		t.Fatal("ID задачи не получен в ответе")
	}
	t.Logf("Создана задача с ID: %s", taskID)

	createdTaskIDOne = taskID
}

func TestRetryMiddleware(t *testing.T) {
	var callCount int32

	// handler, который 2 раза возвращает ошибку, а потом успех
	handler := func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&callCount, 1)
		if c < 3 {
			http.Error(w, "temporary error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}

	ts := httptest.NewServer(resilience.Middleware(handler))
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if callCount != 3 {
		t.Errorf("expected 3 attempts (2 fails + 1 success), got %d", callCount)
	}
}

// ---- тест на circuit breaker ----
func TestCircuitBreakerOpens(t *testing.T) {
	var callCount int32

	handler := func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		http.Error(w, "always fail", http.StatusInternalServerError)
	}

	ts := httptest.NewServer(resilience.Middleware(handler))
	defer ts.Close()

	// Делаем несколько запросов подряд, чтобы CB открылся
	for i := 0; i < 10; i++ {
		resp, _ := http.Get(ts.URL)
		if resp != nil {
			resp.Body.Close()
		}
	}

	if callCount < 5 {
		t.Errorf("expected at least 5 handler calls before CB opens, got %d", callCount)
	}
}

// ---- тест на rate limiter ----
func TestRateLimiterBlocksExcessRequests(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	ts := httptest.NewServer(resilience.Middleware(handler))
	defer ts.Close()

	var tooMany int
	for i := 0; i < 20; i++ {
		resp, err := http.Get(ts.URL)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			tooMany++
		}
		resp.Body.Close()
	}

	if tooMany == 0 {
		t.Errorf("expected some requests to be rate-limited, got 0")
	}
}

// ---- тест на общее поведение ----
func TestMiddlewareHandlesSuccessfulRequests(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}

	ts := httptest.NewServer(resilience.Middleware(handler))
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}
