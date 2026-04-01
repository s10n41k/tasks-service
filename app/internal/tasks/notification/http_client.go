package notification

import (
	"TODOLIST_Tasks/app/internal/tasks/port"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
)

type httpNotificationClient struct {
	gatewayHost  string
	gatewayPort  string
	notifySecret string
	httpClient   *http.Client
}

// NewHTTPClient создаёт HTTP клиент для отправки уведомлений через gateway /internal/notify.
func NewHTTPClient(gatewayHost, gatewayPort, notifySecret string) port.NotificationClient {
	return &httpNotificationClient{
		gatewayHost:  gatewayHost,
		gatewayPort:  gatewayPort,
		notifySecret: notifySecret,
		httpClient:   &http.Client{},
	}
}

func (c *httpNotificationClient) SendReminder(ctx context.Context, n port.ReminderNotification) error {
	body, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("marshal reminder notification: %w", err)
	}

	// HMAC-SHA256 подпись тела запроса с NOTIFY_SECRET
	h := hmac.New(sha256.New, []byte(c.notifySecret))
	h.Write(body)
	sig := hex.EncodeToString(h.Sum(nil))

	target := fmt.Sprintf("http://%s:%s/internal/notify", c.gatewayHost, c.gatewayPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create notify request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Notify-Signature", sig)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send notify request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("notify request failed with status %d", resp.StatusCode)
	}
	return nil
}
