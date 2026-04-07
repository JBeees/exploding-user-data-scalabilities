package middleware

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sony/gobreaker"
	"peak-load-management/metrics"
)

var (
	dbCircuitBreaker    *gobreaker.CircuitBreaker
	cacheCircuitBreaker *gobreaker.CircuitBreaker
)

func InitCircuitBreakers() {
	maxRequests, _ := strconv.Atoi(getEnv("CB_MAX_REQUESTS", "5"))
	intervalSec, _ := strconv.Atoi(getEnv("CB_INTERVAL_SECONDS", "60"))
	timeoutSec, _ := strconv.Atoi(getEnv("CB_TIMEOUT_SECONDS", "30"))
	failureThreshold, _ := strconv.Atoi(getEnv("CB_FAILURE_THRESHOLD", "5"))

	settings := func(name string) gobreaker.Settings {
		return gobreaker.Settings{
			Name:        name,
			MaxRequests: uint32(maxRequests),
			Interval:    time.Duration(intervalSec) * time.Second,
			Timeout:     time.Duration(timeoutSec) * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				// Buka circuit jika failure >= threshold
				return int(counts.ConsecutiveFailures) >= failureThreshold
			},
			OnStateChange: func(name string, from, to gobreaker.State) {
				log.Printf("⚡ Circuit breaker [%s]: %s → %s", name, from, to)
				// Update Prometheus metric
				switch to {
				case gobreaker.StateClosed:
					metrics.CircuitBreakerState.WithLabelValues(name).Set(0)
				case gobreaker.StateHalfOpen:
					metrics.CircuitBreakerState.WithLabelValues(name).Set(1)
				case gobreaker.StateOpen:
					metrics.CircuitBreakerState.WithLabelValues(name).Set(2)
				}
			},
		}
	}

	dbCircuitBreaker = gobreaker.NewCircuitBreaker(settings("database"))
	cacheCircuitBreaker = gobreaker.NewCircuitBreaker(settings("cache"))

	// Init metrics to 0 (closed state)
	metrics.CircuitBreakerState.WithLabelValues("database").Set(0)
	metrics.CircuitBreakerState.WithLabelValues("cache").Set(0)

	log.Println("✅ Circuit breakers initialized")
}

// GetDBCircuitBreaker returns the database circuit breaker
func GetDBCircuitBreaker() *gobreaker.CircuitBreaker {
	return dbCircuitBreaker
}

// GetCacheCircuitBreaker returns the cache circuit breaker
func GetCacheCircuitBreaker() *gobreaker.CircuitBreaker {
	return cacheCircuitBreaker
}

// CircuitBreakerMiddleware wraps handler dengan circuit breaker check
func CircuitBreakerMiddleware(cb *gobreaker.CircuitBreaker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if os.Getenv("ENV") != "test" && cb.State() == gobreaker.StateOpen {
			metrics.CircuitBreakerState.WithLabelValues(cb.Name()).Set(2)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":    "service temporarily unavailable",
				"message":  "Circuit breaker is open. Service is recovering.",
				"trace_id": c.GetString("trace_id"),
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// ExecuteWithCB menjalankan fungsi dalam circuit breaker
func ExecuteWithCB(cb *gobreaker.CircuitBreaker, fn func() (interface{}, error)) (interface{}, error) {
	return cb.Execute(func() (interface{}, error) {
		return fn()
	})
}
