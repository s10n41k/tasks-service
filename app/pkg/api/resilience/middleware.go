package resilience

import (
	"bytes"
	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"
	"io"
	"log"
	"net/http"
	"time"
)

const maxRetries = 3
const baseDelay = 50 * time.Millisecond

func Middleware(handlerFunc http.HandlerFunc) http.HandlerFunc {
	cbSettings := gobreaker.Settings{
		Name:        "global-circuit-breaker",
		MaxRequests: 5,
		Timeout:     5 * time.Second,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			failRatio := float64(c.TotalFailures) / float64(c.Requests)
			return c.Requests >= 5 && failRatio >= 0.5
		},
	}
	cb := gobreaker.NewCircuitBreaker(cbSettings)
	limiter := rate.NewLimiter(rate.Limit(10), 10)

	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}

		var lastErr error

		for attempt := 0; attempt < maxRetries; attempt++ {
			buf := &bytes.Buffer{}
			rec := &responseRecorder{ResponseWriter: w, body: buf, status: http.StatusOK}

			_, err := cb.Execute(func() (interface{}, error) {
				handlerFunc(rec, r)
				return nil, nil
			})

			if err == nil && rec.status < 500 {
				w.WriteHeader(rec.status)
				_, _ = io.Copy(w, bytes.NewReader(buf.Bytes()))
				return
			}

			lastErr = err
			log.Printf("[Middleware] Attempt %d/%d failed: %v", attempt+1, maxRetries, err)
			time.Sleep(time.Duration(1<<attempt) * baseDelay)
		}

		http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
		log.Printf("[Middleware] Handler failed after %d retries: %v", maxRetries, lastErr)
	}
}

type responseRecorder struct {
	http.ResponseWriter
	body   *bytes.Buffer
	status int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}
