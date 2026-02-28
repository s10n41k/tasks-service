package worker

import (
	"TODOLIST_Tasks/app/internal/tasks/port"
	"TODOLIST_Tasks/app/internal/tasks/repository/outbox"
	"TODOLIST_Tasks/app/pkg/logging"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Processor struct {
	outboxRepo   port.OutboxRepository
	kafkaRepo    port.KafkaRepository
	logger       logging.Logger
	batchSize    int
	pollInterval time.Duration
	maxAttempts  int
}

func NewProcessor(
	outboxRepo port.OutboxRepository,
	kafkaRepo port.KafkaRepository,
	logger logging.Logger,
) *Processor {
	return &Processor{
		outboxRepo:   outboxRepo,
		kafkaRepo:    kafkaRepo,
		logger:       logger,
		batchSize:    1000,
		pollInterval: 500 * time.Millisecond,
		maxAttempts:  5,
	}
}

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

func (p *Processor) processBatch(ctx context.Context) {
	events, err := p.outboxRepo.GetUnprocessedEvents(ctx, p.batchSize)
	if err != nil {
		p.logger.Errorf("get unprocessed events: %v", err)
		return
	}
	if len(events) == 0 {
		return
	}

	p.logger.Infof("processing %d outbox events", len(events))

	var successIDs []string
	for _, event := range events {
		if err := p.processEvent(ctx, event); err != nil {
			p.logger.Errorf("process event %s: %v", event.ID, err)
			if event.Attempts < p.maxAttempts {
				if markErr := p.outboxRepo.MarkAsFailed(ctx, event.ID, err.Error()); markErr != nil {
					p.logger.Errorf("mark failed %s: %v", event.ID, markErr)
				}
			} else {
				p.logger.Warnf("event %s exceeded max attempts (%d), skipping", event.ID, p.maxAttempts)
			}
		} else {
			successIDs = append(successIDs, event.ID)
		}
	}

	if len(successIDs) > 0 {
		if err := p.outboxRepo.MarkBatchAsProcessed(ctx, successIDs); err != nil {
			p.logger.Errorf("mark batch processed: %v", err)
		}
	}
}

func (p *Processor) processEvent(ctx context.Context, event port.OutboxEvent) error {
	var payload outbox.TaskPayload
	if err := json.Unmarshal(event.EventData, &payload); err != nil {
		return fmt.Errorf("unmarshal task payload: %w", err)
	}

	task := outbox.PayloadToTask(payload)

	if err := p.kafkaRepo.Write(ctx, task, event.EventType); err != nil {
		return fmt.Errorf("kafka write: %w", err)
	}

	p.logger.Infof("sent %s event for task %s", event.EventType, task.ID)
	return nil
}
