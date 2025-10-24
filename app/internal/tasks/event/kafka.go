package event

import (
	"TODOLIST_Tasks/app/internal/tasks/model"
	kafkaclient "TODOLIST_Tasks/app/pkg/client/kafka"
	"context"
	"encoding/json"
)

type repository struct {
	client *kafkaclient.Client
	topic  string
	key    string
}

func NewRepository(client *kafkaclient.Client, topic string, key string) Repository {
	return &repository{client: client, topic: topic, key: key}
}

func (r *repository) TaskCreated(ctx context.Context, task model.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return r.client.Produce(ctx, r.topic, nil, data)
}

func (r *repository) TaskUpdated(ctx context.Context, task model.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return r.client.Produce(ctx, r.topic, nil, data)
}

func (r *repository) TaskDeleted(ctx context.Context, id string) error {
	data, err := json.Marshal(map[string]string{"id": id})
	if err != nil {
		return err
	}
	return r.client.Produce(ctx, r.topic, nil, data)
}
