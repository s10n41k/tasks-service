package worker

import (
	"TODOLIST_Tasks/app/internal/tasks/event/kafka"
	"TODOLIST_Tasks/app/internal/tasks/model"
	"TODOLIST_Tasks/app/internal/tasks/storage/outbox"
	"TODOLIST_Tasks/app/pkg/logging"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Processor struct {
	outboxRepo   outbox.Repository
	kafkaRepo    kafka.Repository
	logger       logging.Logger
	batchSize    int
	pollInterval time.Duration
	maxAttempts  int
}

func NewProcessor(
	outboxRepo outbox.Repository,
	kafkaRepo kafka.Repository,
	logger logging.Logger,
) *Processor {
	return &Processor{
		outboxRepo:   outboxRepo,
		kafkaRepo:    kafkaRepo,
		logger:       logger,
		batchSize:    100,
		pollInterval: 10 * time.Second,
		maxAttempts:  5,
	}
}

// Start запускает outbox worker
func (p *Processor) Start(ctx context.Context) {
	p.logger.Info("Outbox processor started")

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("Outbox processor stopped")
			return
		case <-ticker.C:
			p.processBatch(ctx)
		}
	}
}

// processBatch обрабатывает пачку необработанных событий
func (p *Processor) processBatch(ctx context.Context) {
	events, err := p.outboxRepo.GetUnprocessedEvents(ctx, p.batchSize)
	if err != nil {
		p.logger.Errorf("Failed to get unprocessed events: %v", err)
		return
	}

	if len(events) == 0 {
		return
	}

	p.logger.Infof("Processing %d outbox events", len(events))

	for _, event := range events {
		if err := p.processEvent(ctx, event); err != nil {
			p.logger.Errorf("Failed to process event %s: %v", event.ID, err)

			if event.Attempts < p.maxAttempts {
				if markErr := p.outboxRepo.MarkAsFailed(ctx, event.ID, err.Error()); markErr != nil {
					p.logger.Errorf("Failed to mark event as failed: %v", markErr)
				}
			} else {
				p.logger.Warnf("Event %s exceeded max attempts (%d), skipping", event.ID, p.maxAttempts)
			}
		} else {
			if markErr := p.outboxRepo.MarkAsProcessed(ctx, event.ID); markErr != nil {
				p.logger.Errorf("Failed to mark event as processed: %v", markErr)
			}
		}
	}
}

// processEvent обрабатывает одно событие
func (p *Processor) processEvent(ctx context.Context, event model.Event) error {
	p.logger.Debugf("Processing event %s for task %s (type: %s)",
		event.ID, event.AggregateID, event.EventType)

	// Десериализуем данные задачи
	var task model.Task
	if err := json.Unmarshal(event.EventData, &task); err != nil {
		return fmt.Errorf("failed to unmarshal task data: %w", err)
	}

	// Отправляем в Kafka через твой Repository
	if err := p.kafkaRepo.Write(ctx, task, event.EventType); err != nil {
		return fmt.Errorf("failed to send to kafka: %w", err)
	}

	p.logger.Infof("Successfully sent %s event for task %s to Kafka", event.EventType, task.Id)
	return nil
}
