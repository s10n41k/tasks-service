package redis

import (
	"TODOLIST_Tasks/app/internal/config"
	"TODOLIST_Tasks/app/pkg/utils/repeatable"
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"time"
)

type Client interface {
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd
	SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	LRange(ctx context.Context, key string, start, stop int64) *redis.StringSliceCmd
	RPush(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	LPop(ctx context.Context, key string) *redis.StringCmd
	RPop(ctx context.Context, key string) *redis.StringCmd
	Exists(ctx context.Context, keys ...string) *redis.IntCmd
	SMembers(ctx context.Context, key string) *redis.StringSliceCmd
}

func NewClient(ctx context.Context, maxAttempts int, sc config.StorageRedisTasks) (client *redis.Client, err error) {

	// Попытки подключиться с повторениями в случае неудачи
	err = repeatable.DoWithTries(func() error {
		// Создаем новый контекст с таймаутом
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		// Создаем новый Redis клиент
		client = redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%s", sc.Host, sc.Port),
			Password: sc.Password,
			DB:       0,
		})

		_, err := client.Ping(ctx).Result()
		if err != nil {
			return err
		}
		return nil
	}, maxAttempts, 5*time.Second)

	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis after %d attempts: %w", maxAttempts, err)
	}

	return client, nil
}
