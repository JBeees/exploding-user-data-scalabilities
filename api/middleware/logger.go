package middleware

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"peak-load-management/metrics"
)

// LogEntry struktur JSON log untuk setiap request
type LogEntry struct {
	Timestamp  string  `json:"timestamp"`
	TraceID    string  `json:"trace_id"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	StatusCode int     `json:"status_code"`
	LatencyMs  float64 `json:"latency_ms"`
	ClientIP   string  `json:"client_ip"`
	UserAgent  string  `json:"user_agent"`
	Error      string  `json:"error,omitempty"`
}

// Logger middleware — structured JSON logging + Prometheus metrics
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		latency := time.Since(start)
		latencyMs := float64(latency.Milliseconds())
		status := fmt.Sprintf("%d", c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		// Update Prometheus metrics
		metrics.HttpRequestsTotal.
			WithLabelValues(c.Request.Method, path, status).
			Inc()

		metrics.HttpRequestDuration.
			WithLabelValues(c.Request.Method, path).
			Observe(latencyMs)

		// Structured JSON log
		entry := LogEntry{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			TraceID:    c.GetString("trace_id"),
			Method:     c.Request.Method,
			Path:       path,
			StatusCode: c.Writer.Status(),
			LatencyMs:  latencyMs,
			ClientIP:   c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
		}

		if len(c.Errors) > 0 {
			entry.Error = c.Errors.String()
		}

		logJSON, _ := json.Marshal(entry)
		log.Println(string(logJSON))
	}
}
