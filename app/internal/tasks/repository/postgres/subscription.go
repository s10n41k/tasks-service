package postgres

import (
	"TODOLIST_Tasks/app/internal/tasks/port"
	postgresql "TODOLIST_Tasks/app/pkg/client/postgres"
	"TODOLIST_Tasks/app/pkg/logging"
	"context"
	"database/sql"
	"fmt"
	"time"
)

type subscriptionRepo struct {
	db     postgresql.Client
	logger logging.Logger
}

func NewSubscriptionRepository(db postgresql.Client, logger logging.Logger) port.SubscriptionRepository {
	return &subscriptionRepo{db: db, logger: logger}
}

func (r *subscriptionRepo) UpsertSubscription(ctx context.Context, sub port.UserSubscription) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_subscriptions (user_id, name, has_subscription, expires_at, telegram_chat_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			name             = CASE WHEN EXCLUDED.name != '' THEN EXCLUDED.name ELSE user_subscriptions.name END,
			has_subscription = EXCLUDED.has_subscription,
			expires_at       = EXCLUDED.expires_at,
			telegram_chat_id = COALESCE(EXCLUDED.telegram_chat_id, user_subscriptions.telegram_chat_id),
			updated_at       = NOW()`,
		sub.UserID, sub.Name, sub.HasSubscription, sub.ExpiresAt, sub.TelegramChatID,
	)
	if err != nil {
		r.logger.Errorf("UpsertSubscription user %s: %v", sub.UserID, err)
		return fmt.Errorf("upsert subscription: %w", err)
	}
	return nil
}

func (r *subscriptionRepo) GetSubscription(ctx context.Context, userID string) (*port.UserSubscription, error) {
	var sub port.UserSubscription
	err := r.db.QueryRow(ctx, `
		SELECT user_id, name, has_subscription, expires_at, telegram_chat_id, updated_at
		FROM user_subscriptions WHERE user_id = $1`, userID,
	).Scan(&sub.UserID, &sub.Name, &sub.HasSubscription, &sub.ExpiresAt, &sub.TelegramChatID, &sub.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		r.logger.Errorf("GetSubscription user %s: %v", userID, err)
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	return &sub, nil
}

func (r *subscriptionRepo) FindTasksDueForReminder(ctx context.Context, from, to time.Time, window port.ReminderWindow) ([]port.TaskReminderInfo, error) {
	sentCol := reminderSentCol(window)
	q := fmt.Sprintf(`
		SELECT t.task_id, t.title, t.due_date, t.user_id, s.telegram_chat_id
		FROM tasks t
		JOIN user_subscriptions s ON s.user_id = t.user_id
		WHERE t.status != 3
		  AND s.has_subscription = TRUE
		  AND s.expires_at > NOW()
		  AND s.telegram_chat_id IS NOT NULL
		  AND t.due_date BETWEEN $1 AND $2
		  AND t.%s IS NULL`, sentCol)

	rows, err := r.db.Query(ctx, q, from, to)
	if err != nil {
		r.logger.Errorf("FindTasksDueForReminder window %d: %v", window, err)
		return nil, fmt.Errorf("find tasks due for reminder: %w", err)
	}
	defer rows.Close()

	var result []port.TaskReminderInfo
	for rows.Next() {
		var info port.TaskReminderInfo
		if err := rows.Scan(&info.TaskID, &info.Title, &info.DueDate, &info.UserID, &info.TelegramChatID); err != nil {
			r.logger.Errorf("FindTasksDueForReminder scan: %v", err)
			return nil, fmt.Errorf("scan reminder row: %w", err)
		}
		result = append(result, info)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return result, nil
}

func (r *subscriptionRepo) MarkReminderSent(ctx context.Context, taskID string, window port.ReminderWindow) error {
	sentCol := reminderSentCol(window)
	q := fmt.Sprintf(`UPDATE tasks SET %s = NOW() WHERE task_id = $1`, sentCol)
	_, err := r.db.Exec(ctx, q, taskID)
	if err != nil {
		r.logger.Errorf("MarkReminderSent task %s window %d: %v", taskID, window, err)
		return fmt.Errorf("mark reminder sent: %w", err)
	}
	return nil
}

func reminderSentCol(window port.ReminderWindow) string {
	switch window {
	case port.Window60m:
		return "reminder_60m_sent_at"
	case port.Window15m:
		return "reminder_15m_sent_at"
	case port.Window5m:
		return "reminder_5m_sent_at"
	default:
		return "reminder_60m_sent_at"
	}
}
