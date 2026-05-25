package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests, partitioned by method, route, and status code.",
	}, []string{"method", "route", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	httpRequestsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Current number of HTTP requests being processed.",
	})
)

// MetricsMiddleware records Prometheus metrics for every HTTP request.
// It must run inside the OTel tracing middleware so route patterns are resolved.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpRequestsInFlight.Inc()
		defer httpRequestsInFlight.Dec()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)

		// Prefer the matched chi route pattern (e.g. /media/{id}) over the raw
		// URL path to avoid a label explosion from dynamic path segments.
		route := r.URL.Path
		if chiCtx := chi.RouteContext(r.Context()); chiCtx != nil {
			if p := chiCtx.RoutePattern(); p != "" {
				route = p
			}
		}

		httpRequestsTotal.
			WithLabelValues(r.Method, route, strconv.Itoa(ww.Status())).
			Inc()
		httpRequestDuration.
			WithLabelValues(r.Method, route).
			Observe(time.Since(start).Seconds())
	})
}
