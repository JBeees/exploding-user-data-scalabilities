package middleware

import (
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
	"peak-load-management/metrics"
)

// ipLimiter menyimpan rate limiter per IP
type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	limiters = make(map[string]*ipLimiter)
	mu       sync.Mutex
)

func init() {
	// Cleanup stale IP limiters every 5 minutes
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			cleanupLimiters()
		}
	}()
}

func getLimiter(ip string) *rate.Limiter {
	mu.Lock()
	defer mu.Unlock()

	if l, exists := limiters[ip]; exists {
		l.lastSeen = time.Now()
		return l.limiter
	}

	rps, _ := strconv.Atoi(getEnv("RATE_LIMIT_RPS", "500"))
	burst, _ := strconv.Atoi(getEnv("RATE_LIMIT_BURST", "100"))

	l := rate.NewLimiter(rate.Limit(rps), burst)
	limiters[ip] = &ipLimiter{limiter: l, lastSeen: time.Now()}
	return l
}

func cleanupLimiters() {
	mu.Lock()
	defer mu.Unlock()
	for ip, l := range limiters {
		if time.Since(l.lastSeen) > 10*time.Minute {
			delete(limiters, ip)
		}
	}
}

// RateLimit middleware membatasi request per IP
func RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := getLimiter(ip)

		if !limiter.Allow() {
			metrics.RateLimitRejectedTotal.Inc()
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":      "rate limit exceeded",
				"message":    "Too many requests. Please slow down.",
				"retry_after": "1s",
				"trace_id":   c.GetString("trace_id"),
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
