package notification

import (
	"TODOLIST_Tasks/app/internal/tasks/port"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type wsNotifier struct {
	usersServiceURL string
	httpClient      *http.Client
}

// NewWsNotifier создаёт HTTP-клиент для отправки WS-уведомлений через users-service.
func NewWsNotifier(host, servicePort string) port.WsNotifier {
	return &wsNotifier{
		usersServiceURL: "http://" + host + ":" + servicePort,
		httpClient:      &http.Client{},
	}
}

func (n *wsNotifier) Notify(ctx context.Context, userID, eventType string, data interface{}) error {
	body, err := json.Marshal(map[string]interface{}{
		"user_id":    userID,
		"event_type": eventType,
		"data":       data,
	})
	if err != nil {
		return fmt.Errorf("marshal ws notify: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.usersServiceURL+"/internal/ws-notify", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create ws notify request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send ws notify user %s event %s: %w", userID, eventType, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ws notify failed with status %d", resp.StatusCode)
	}
	return nil
}
