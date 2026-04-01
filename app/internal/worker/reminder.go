package worker

import (
	"TODOLIST_Tasks/app/internal/tasks/port"
	"TODOLIST_Tasks/app/pkg/logging"
	"context"
	"time"
)

type ReminderWorker struct {
	subRepo      port.SubscriptionRepository
	notifyClient port.NotificationClient
	logger       logging.Logger
	pollInterval time.Duration
}

func NewReminderWorker(
	subRepo port.SubscriptionRepository,
	notifyClient port.NotificationClient,
	logger logging.Logger,
) *ReminderWorker {
	return &ReminderWorker{
		subRepo:      subRepo,
		notifyClient: notifyClient,
		logger:       logger,
		pollInterval: time.Minute,
	}
}

func (w *ReminderWorker) Start(ctx context.Context) {
	w.logger.Info("Reminder worker started")
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Reminder worker stopped")
			return
		case <-ticker.C:
			w.processWindow(ctx, port.Window60m, 60*time.Minute, "60m")
			w.processWindow(ctx, port.Window15m, 15*time.Minute, "15m")
			w.processWindow(ctx, port.Window5m, 5*time.Minute, "5m")
		}
	}
}

func (w *ReminderWorker) processWindow(ctx context.Context, window port.ReminderWindow, ahead time.Duration, label string) {
	now := time.Now()
	from := now.Add(ahead - time.Minute)
	to := now.Add(ahead + time.Minute)

	tasks, err := w.subRepo.FindTasksDueForReminder(ctx, from, to, window)
	if err != nil {
		w.logger.Errorf("reminder worker find tasks window %s: %v", label, err)
		return
	}

	for _, t := range tasks {
		n := port.ReminderNotification{
			UserID:         t.UserID,
			TelegramChatID: t.TelegramChatID,
			TaskID:         t.TaskID,
			TaskTitle:      t.Title,
			DueDate:        t.DueDate,
			ReminderType:   label,
		}
		if err := w.notifyClient.SendReminder(ctx, n); err != nil {
			w.logger.Errorf("send reminder task %s window %s: %v", t.TaskID, label, err)
			continue
		}
		if err := w.subRepo.MarkReminderSent(ctx, t.TaskID, window); err != nil {
			w.logger.Errorf("mark reminder sent task %s window %s: %v", t.TaskID, label, err)
		}
	}
}
