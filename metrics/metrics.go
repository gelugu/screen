package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var Registry = prometheus.NewRegistry()

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

var (
	// RequestsTotal counts every request entering the proxy.
	// result: proxied | challenged | not_found
	RequestsTotal = promauto.With(Registry).NewCounterVec(
		prometheus.CounterOpts{
			Name: "screen_requests_total",
			Help: "Total number of requests handled by the proxy.",
		},
		[]string{"domain", "method", "result"},
	)

	// RequestDuration measures end-to-end handling time per request.
	RequestDuration = promauto.With(Registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "screen_request_duration_seconds",
			Help:    "End-to-end request handling duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"domain", "method"},
	)

	// ChallengesTotal tracks challenge lifecycle events.
	// result: issued | passed | failed
	ChallengesTotal = promauto.With(Registry).NewCounterVec(
		prometheus.CounterOpts{
			Name: "screen_challenges_total",
			Help: "Total number of challenge events by type and result.",
		},
		[]string{"domain", "type", "result"},
	)

	// RecaptchaScores tracks the distribution of reCAPTCHA v3 scores.
	// result: pass | fail
	RecaptchaScores = promauto.With(Registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "screen_recaptcha_score",
			Help:    "Distribution of reCAPTCHA v3 scores received.",
			Buckets: []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
		},
		[]string{"result"},
	)

	// RecaptchaErrorsTotal counts network/parse errors calling the reCAPTCHA API.
	RecaptchaErrorsTotal = promauto.With(Registry).NewCounter(
		prometheus.CounterOpts{
			Name: "screen_recaptcha_errors_total",
			Help: "Total number of errors communicating with the reCAPTCHA API.",
		},
	)

	// UpstreamErrorsTotal counts errors returned by upstream proxied services.
	UpstreamErrorsTotal = promauto.With(Registry).NewCounterVec(
		prometheus.CounterOpts{
			Name: "screen_upstream_errors_total",
			Help: "Total number of upstream proxy errors.",
		},
		[]string{"upstream"},
	)

	// SessionsCreatedTotal counts all sessions ever created.
	SessionsCreatedTotal = promauto.With(Registry).NewCounter(
		prometheus.CounterOpts{
			Name: "screen_sessions_created_total",
			Help: "Total number of sessions created since startup.",
		},
	)

	// SessionsActive is the current number of in-memory sessions.
	SessionsActive = promauto.With(Registry).NewGauge(
		prometheus.GaugeOpts{
			Name: "screen_sessions_active",
			Help: "Number of currently active in-memory sessions.",
		},
	)
)
