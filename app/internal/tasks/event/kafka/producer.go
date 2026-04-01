package kafka

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/repository/outbox"
	kafkaclient "TODOLIST_Tasks/app/pkg/client/kafka"
	"TODOLIST_Tasks/app/pkg/logging"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type kafkaPayload struct {
	Task      outbox.TaskPayload `json:"task"`
	Type      string             `json:"type"`
	Timestamp time.Time          `json:"timestamp"`
}

type producer struct {
	client kafkaclient.Client
	logger logging.Logger
}

func NewRepository(client kafkaclient.Client) Repository {
	return &producer{
		client: client,
		logger: *logging.GetLogger().GetLoggerWithField("component", "kafka-producer"),
	}
}

func (p *producer) Write(ctx context.Context, task domain.Task, eventType string) error {
	payload := kafkaPayload{
		Task: outbox.TaskPayload{
			ID:          task.ID,
			Title:       task.Title,
			Description: task.Description,
			Priority:    string(task.Priority),
			Status:      string(task.Status),
			DueDate:     task.DueDate,
			UserID:      task.UserID,
			TagID:       task.TagID,
			TagName:     task.TagName,
			CreatedAt:   task.CreatedAt,
		},
		Type:      eventType,
		Timestamp: time.Now().UTC(),
	}

	p.logger.Infof("producing %s event for task %s", eventType, task.ID)

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal kafka event: %w", err)
	}
	if err := p.client.Write(ctx, task.ID, data); err != nil {
		return fmt.Errorf("kafka write: %w", err)
	}

	p.logger.Infof("sent %s event for task %s", eventType, task.ID)
	return nil
}

func (p *producer) Close() error {
	return p.client.Close()
}
