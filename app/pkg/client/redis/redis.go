package redis

import (
	"TODOLIST_Tasks/app/internal/config"
	"TODOLIST_Tasks/app/pkg/utils/repeatable"
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
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
	Pipeline() redis.Pipeliner
	Close() error
}

// 🔥 Wrapper структура
type clientWrapper struct {
	client *redis.Client
}

func (c *clientWrapper) Close() error {
	return c.client.Close()
}

func (c *clientWrapper) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	return c.client.Eval(ctx, script, keys, args...)
}

func (c *clientWrapper) SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	return c.client.SAdd(ctx, key, members...)
}

func (c *clientWrapper) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	return c.client.Set(ctx, key, value, expiration)
}

func (c *clientWrapper) Get(ctx context.Context, key string) *redis.StringCmd {
	return c.client.Get(ctx, key)
}

func (c *clientWrapper) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	return c.client.Del(ctx, keys...)
}

func (c *clientWrapper) LRange(ctx context.Context, key string, start, stop int64) *redis.StringSliceCmd {
	return c.client.LRange(ctx, key, start, stop)
}

func (c *clientWrapper) RPush(ctx context.Context, key string, values ...interface{}) *redis.IntCmd {
	return c.client.RPush(ctx, key, values...)
}

func (c *clientWrapper) LPop(ctx context.Context, key string) *redis.StringCmd {
	return c.client.LPop(ctx, key)
}

func (c *clientWrapper) RPop(ctx context.Context, key string) *redis.StringCmd {
	return c.client.RPop(ctx, key)
}

func (c *clientWrapper) Exists(ctx context.Context, keys ...string) *redis.IntCmd {
	return c.client.Exists(ctx, keys...)
}

func (c *clientWrapper) SMembers(ctx context.Context, key string) *redis.StringSliceCmd {
	return c.client.SMembers(ctx, key)
}

// 🔥 ВОТ ЭТО НОВОЕ
func (c *clientWrapper) Pipeline() redis.Pipeliner {
	return c.client.Pipeline()
}

func NewClient(ctx context.Context, maxAttempts int, sc config.Config) (Client, error) { // 🔥 ИЗМЕНИЛ ТИП ВОЗВРАТА
	var redisClient *redis.Client
	var err error

	err = repeatable.DoWithTries(func() error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		redisClient = redis.NewClient(&redis.Options{
			Addr:         fmt.Sprintf("%s:%s", sc.Redis.Host, sc.Redis.Port),
			Password:     sc.Redis.Password,
			DB:           0,
			PoolSize:     1000,
			MinIdleConns: 50,
		})

		_, err := redisClient.Ping(ctx).Result()
		return err
	}, maxAttempts, 5*time.Second)

	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis after %d attempts: %w", maxAttempts, err)
	}

	// 🔥 ВОЗВРАЩАЕМ Wrapper вместо *redis.Client
	return &clientWrapper{client: redisClient}, nil
}
