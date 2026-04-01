package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// UsersMessageClient — клиент для отправки сообщений через users-service.
// Используется tasks-service для авто-сообщений при изменении/удалении совместных задач.
type UsersMessageClient struct {
	usersHost  string
	usersPort  string
	httpClient *http.Client
}

func NewUsersMessageClient(usersHost, usersPort string) *UsersMessageClient {
	return &UsersMessageClient{
		usersHost:  usersHost,
		usersPort:  usersPort,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// SendMessage отправляет сообщение от fromUserID к toUserID через users-service.
// Signature middleware users-service пропускает запросы с X-Service-Name: tasks-service без проверки подписи.
func (c *UsersMessageClient) SendMessage(ctx context.Context, fromUserID, toUserID, content string) error {
	body, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return fmt.Errorf("marshal message body: %w", err)
	}

	url := fmt.Sprintf("http://%s:%s/users/%s/messages/%s", c.usersHost, c.usersPort, fromUserID, toUserID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create message request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Name", "tasks-service")
	req.Header.Set("X-User-ID", fromUserID)
	req.Header.Set("X-Is-System", "true")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send message request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("message request failed with status %d", resp.StatusCode)
	}
	return nil
}
