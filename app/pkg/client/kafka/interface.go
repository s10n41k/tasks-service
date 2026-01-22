package kafka

import "context"

type Client interface {
	Write(ctx context.Context, key string, msg []byte) error
	Close() error
}
