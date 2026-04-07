package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP metrics
	HttpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests",
	}, []string{"method", "path", "status"})

	HttpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_ms",
		Help:    "HTTP request duration in milliseconds",
		Buckets: []float64{10, 50, 100, 200, 500, 1000, 2000, 5000, 10000},
	}, []string{"method", "path"})

	// Cache metrics
	CacheHitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cache_hits_total",
		Help: "Total cache hits",
	}, []string{"key_type"})

	CacheMissesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cache_misses_total",
		Help: "Total cache misses",
	}, []string{"key_type"})

	// Queue metrics
	QueueMessagesPending = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "queue_messages_pending",
		Help: "Current number of pending messages in queue",
	})

	QueuePublishedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "queue_published_total",
		Help: "Total messages published to queue",
	})

	QueueProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "queue_processed_total",
		Help: "Total messages processed from queue",
	}, []string{"status"})

	// Rate limit metrics
	RateLimitRejectedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rate_limit_rejected_total",
		Help: "Total requests rejected by rate limiter",
	})

	// Circuit breaker metrics
	CircuitBreakerState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "circuit_breaker_state",
		Help: "Circuit breaker state (0=closed, 1=half-open, 2=open)",
	}, []string{"name"})

	// DB metrics
	DbConnectionsActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "db_connections_active",
		Help: "Active database connections",
	}, []string{"type"})

	// Transaction metrics
	TransactionsCreatedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "transactions_created_total",
		Help: "Total transactions created",
	}, []string{"type", "status"})
)
