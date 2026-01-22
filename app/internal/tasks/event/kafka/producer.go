package kafka

import (
	"TODOLIST_Tasks/app/internal/tasks/model"
	"context"
	"encoding/json"
	"fmt"
	"time"

	kafkaclient "TODOLIST_Tasks/app/pkg/client/kafka"
	"TODOLIST_Tasks/app/pkg/logging"
)

const (
	TypeSave   = "save"
	TypeUpdate = "update"
	TypeDelete = "delete"
)

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

func (p *producer) Write(ctx context.Context, task model.Task, eventType string) error {
	evt := model.EventTask{
		Task:      task,
		Type:      eventType,
		Timestamp: time.Now().UTC(),
	}

	p.logger.Infof("Producing %s event for task %s", eventType, task.Id)

	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal event failed: %w", err)
	}

	if err := p.client.Write(ctx, task.Id, data); err != nil {
		return fmt.Errorf("kafka write failed: %w", err)
	}

	p.logger.Infof("Successfully produced %s event for task %s", eventType, task.Id)
	return nil
}

func (p *producer) Close() error {
	return p.client.Close()
}
