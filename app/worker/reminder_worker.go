package worker

import (
	"TODOLIST_Tasks/app/internal/tasks/port"
	"TODOLIST_Tasks/app/pkg/logging"
	"context"
	"sync"
	"time"
)

// reminderWindow описывает одно временное окно напоминания.
type reminderWindow struct {
	name     string
	window   port.ReminderWindow
	minBound time.Duration
	maxBound time.Duration
}

var reminderWindows = []reminderWindow{
	{name: "60m", window: port.Window60m, minBound: 55 * time.Minute, maxBound: 65 * time.Minute},
	{name: "15m", window: port.Window15m, minBound: 12 * time.Minute, maxBound: 18 * time.Minute},
	{name: "5m", window: port.Window5m, minBound: 3 * time.Minute, maxBound: 7 * time.Minute},
}

// ReminderWorker — воркер напоминаний о задачах для пользователей с активной подпиской.
type ReminderWorker struct {
	subRepo  port.SubscriptionRepository
	notifier port.NotificationClient
	logger   logging.Logger
}

func NewReminderWorker(subRepo port.SubscriptionRepository, notifier port.NotificationClient, logger logging.Logger) *ReminderWorker {
	return &ReminderWorker{
		subRepo:  subRepo,
		notifier: notifier,
		logger:   logger,
	}
}

// Start запускает тикер раз в минуту и параллельно обрабатывает три окна напоминаний.
func (w *ReminderWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	w.logger.Info("reminder worker started")

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("reminder worker stopped")
			return
		case <-ticker.C:
			w.processTick(ctx)
		}
	}
}

func (w *ReminderWorker) processTick(ctx context.Context) {
	now := time.Now()
	var wg sync.WaitGroup

	for _, win := range reminderWindows {
		wg.Add(1)
		go func(win reminderWindow) {
			defer wg.Done()
			from := now.Add(win.minBound)
			to := now.Add(win.maxBound)
			w.processWindow(ctx, win, from, to)
		}(win)
	}

	wg.Wait()
}

func (w *ReminderWorker) processWindow(ctx context.Context, win reminderWindow, from, to time.Time) {
	tasks, err := w.subRepo.FindTasksDueForReminder(ctx, from, to, win.window)
	if err != nil {
		w.logger.Errorf("reminder worker: find tasks window %s: %v", win.name, err)
		return
	}

	for _, task := range tasks {
		n := port.ReminderNotification{
			UserID:         task.UserID,
			TelegramChatID: task.TelegramChatID,
			TaskID:         task.TaskID,
			TaskTitle:      task.Title,
			DueDate:        task.DueDate,
			ReminderType:   win.name,
		}

		if err := w.notifier.SendReminder(ctx, n); err != nil {
			w.logger.Errorf("reminder worker: send reminder task %s window %s: %v", task.TaskID, win.name, err)
			continue
		}

		if err := w.subRepo.MarkReminderSent(ctx, task.TaskID, win.window); err != nil {
			w.logger.Errorf("reminder worker: mark sent task %s window %s: %v", task.TaskID, win.name, err)
		}
	}
}
