package settingsCb

import (
	"github.com/sony/gobreaker"
	"time"
)

func NewRedisBreaker() *gobreaker.CircuitBreaker {
	cbSettings := gobreaker.Settings{
		Name:     "RedisCB",
		Timeout:  5 * time.Second,  // сколько ждать перед возвратом к нормальной работе
		Interval: 20 * time.Second, // как часто сбрасывать статистику
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3 // если подряд 3 ошибки — отключаем Redis
		},
	}

	return gobreaker.NewCircuitBreaker(cbSettings)
}
