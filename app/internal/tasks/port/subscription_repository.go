package port

import (
	"context"
	"time"
)

// ReminderWindow — временное окно для напоминаний.
type ReminderWindow int

const (
	Window60m ReminderWindow = iota
	Window15m
	Window5m
)

// UserSubscription — локальная копия данных подписки пользователя.
type UserSubscription struct {
	UserID          string
	Name            string // имя пользователя (для совместных задач)
	HasSubscription bool
	ExpiresAt       *time.Time
	TelegramChatID  *int64
	UpdatedAt       time.Time
}

// TaskReminderInfo — данные задачи для отправки напоминания.
type TaskReminderInfo struct {
	TaskID         string
	Title          string
	DueDate        time.Time
	UserID         string
	TelegramChatID int64
}

// SubscriptionRepository — контракт хранилища подписок (локальная таблица в tasks-service).
type SubscriptionRepository interface {
	UpsertSubscription(ctx context.Context, sub UserSubscription) error
	GetSubscription(ctx context.Context, userID string) (*UserSubscription, error)
	FindTasksDueForReminder(ctx context.Context, from, to time.Time, window ReminderWindow) ([]TaskReminderInfo, error)
	MarkReminderSent(ctx context.Context, taskID string, window ReminderWindow) error
}
