package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// NewMiddleware возвращает HTTP middleware и handler для /metrics.
func NewMiddleware(serviceName string) (func(http.Handler) http.Handler, http.Handler) {
	requests := promauto.NewCounterVec(prometheus.CounterOpts{
		Name: serviceName + "_http_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "path", "status"})

	duration := promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    serviceName + "_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
	}, []string{"method", "path"})

	inFlight := promauto.NewGauge(prometheus.GaugeOpts{
		Name: serviceName + "_http_requests_in_flight",
		Help: "Current number of in-flight requests",
	})

	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			inFlight.Inc()
			start := time.Now()

			next.ServeHTTP(rec, r)

			inFlight.Dec()
			requests.WithLabelValues(r.Method, r.URL.Path, strconv.Itoa(rec.status)).Inc()
			duration.WithLabelValues(r.Method, r.URL.Path).Observe(time.Since(start).Seconds())
		})
	}

	return mw, promhttp.Handler()
}
