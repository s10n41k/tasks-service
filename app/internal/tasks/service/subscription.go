package service

import (
	"TODOLIST_Tasks/app/internal/tasks/port"
	"context"
	"time"
)

// SubscriptionService — сервис управления подписками пользователей.
type SubscriptionService interface {
	SyncSubscription(ctx context.Context, sub port.UserSubscription) error
	IsActive(ctx context.Context, userID string) bool
}

type subscriptionService struct {
	repo port.SubscriptionRepository
}

// NewSubscriptionService создаёт сервис подписок на основе репозитория.
func NewSubscriptionService(repo port.SubscriptionRepository) SubscriptionService {
	return &subscriptionService{repo: repo}
}

func (s *subscriptionService) SyncSubscription(ctx context.Context, sub port.UserSubscription) error {
	return s.repo.UpsertSubscription(ctx, sub)
}

// IsActive проверяет наличие активной подписки через локальную таблицу.
// Используется как fallback когда JWT-токен содержит устаревшее значение subscription.
func (s *subscriptionService) IsActive(ctx context.Context, userID string) bool {
	sub, err := s.repo.GetSubscription(ctx, userID)
	if err != nil || sub == nil {
		return false
	}
	return sub.HasSubscription && sub.ExpiresAt != nil && sub.ExpiresAt.After(time.Now())
}
