package resilience

import (
	"bytes"
	"errors"
	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

const maxRetries = 3
const baseDelay = 50 * time.Millisecond

var (
	cbOnce sync.Once
	cb     *gobreaker.CircuitBreaker
)

func getCircuitBreaker() *gobreaker.CircuitBreaker {
	cbOnce.Do(func() {
		cbSettings := gobreaker.Settings{
			Name:        "global-circuit-breaker",
			MaxRequests: 10,
			Timeout:     10 * time.Second,
			ReadyToTrip: func(c gobreaker.Counts) bool {
				failRatio := float64(c.TotalFailures) / float64(c.Requests)
				return c.Requests >= 5 && failRatio >= 0.5
			},
		}
		cb = gobreaker.NewCircuitBreaker(cbSettings)
	})
	return cb
}

func Middleware(handlerFunc http.HandlerFunc) http.HandlerFunc {
	breaker := getCircuitBreaker()
	limiter := rate.NewLimiter(rate.Limit(10000), 10000)

	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
			return
		}

		var lastErr error

		for attempt := 0; attempt < maxRetries; attempt++ {
			buf := &bytes.Buffer{}
			rec := &responseRecorder{
				body:    buf,
				headers: make(http.Header),
				status:  http.StatusOK,
			}

			_, err := breaker.Execute(func() (interface{}, error) {
				handlerFunc(rec, r)
				if rec.status >= 500 {
					return nil, &serverError{status: rec.status}
				}
				return nil, nil
			})

			if err == nil {
				// Copy captured headers to the real response
				for k, vals := range rec.headers {
					for _, v := range vals {
						w.Header().Add(k, v)
					}
				}
				w.WriteHeader(rec.status)
				_, _ = io.Copy(w, bytes.NewReader(buf.Bytes()))
				return
			}

			lastErr = err
			log.Printf("[Resilience] attempt %d/%d failed: %v", attempt+1, maxRetries, err)
			time.Sleep(time.Duration(1<<attempt) * baseDelay)
		}

		http.Error(w, `{"error":"service temporarily unavailable"}`, http.StatusServiceUnavailable)
		log.Printf("[Resilience] handler failed after %d retries: %v", maxRetries, lastErr)
	}
}

// WriteMiddleware applies rate limiting and circuit breaker WITHOUT retries.
// Safe for non-idempotent operations (POST, DELETE) — retrying writes causes duplicates.
func WriteMiddleware(handlerFunc http.HandlerFunc) http.HandlerFunc {
	breaker := getCircuitBreaker()
	limiter := rate.NewLimiter(rate.Limit(10000), 10000)

	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
			return
		}

		buf := &bytes.Buffer{}
		rec := &responseRecorder{
			body:    buf,
			headers: make(http.Header),
			status:  http.StatusOK,
		}

		_, cbErr := breaker.Execute(func() (interface{}, error) {
			handlerFunc(rec, r)
			if rec.status >= 500 {
				return nil, &serverError{status: rec.status}
			}
			return nil, nil
		})

		// Circuit breaker open — handler was NOT called, return 503 immediately
		if errors.Is(cbErr, gobreaker.ErrOpenState) || errors.Is(cbErr, gobreaker.ErrTooManyRequests) {
			http.Error(w, `{"error":"service temporarily unavailable"}`, http.StatusServiceUnavailable)
			log.Printf("[WriteResilience] circuit breaker open, rejecting write request")
			return
		}

		// Handler was called — write its captured response (including 5xx)
		for k, vals := range rec.headers {
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(rec.status)
		_, _ = io.Copy(w, bytes.NewReader(buf.Bytes()))
	}
}

type serverError struct {
	status int
}

func (e *serverError) Error() string {
	return http.StatusText(e.status)
}

type responseRecorder struct {
	body    *bytes.Buffer
	headers http.Header
	status  int
}

func (r *responseRecorder) Header() http.Header {
	return r.headers
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}
