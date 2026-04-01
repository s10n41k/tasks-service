package port

import "context"

// WsNotifier — контракт для отправки WS-уведомлений пользователям через users-service.
type WsNotifier interface {
	Notify(ctx context.Context, userID, eventType string, data interface{}) error
}
