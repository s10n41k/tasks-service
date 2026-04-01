package port

import (
	"context"
	"time"
)

// ReminderNotification — данные для отправки напоминания о задаче.
type ReminderNotification struct {
	UserID         string    `json:"user_id"`
	TelegramChatID int64     `json:"telegram_chat_id"`
	TaskID         string    `json:"task_id"`
	TaskTitle      string    `json:"task_title"`
	DueDate        time.Time `json:"due_date"`
	ReminderType   string    `json:"reminder_type"` // "60m", "15m", "5m"
}

// NotificationClient — контракт клиента для отправки уведомлений через gateway.
type NotificationClient interface {
	SendReminder(ctx context.Context, n ReminderNotification) error
}
