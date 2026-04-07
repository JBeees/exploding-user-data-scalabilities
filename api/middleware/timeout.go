package middleware

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	defaultTimeout = 5 * time.Second
	TraceIDHeader  = "X-Trace-ID"
)

// TraceID generates a simple trace ID for request tracking
func generateTraceID() string {
	return fmt.Sprintf("%d-%x", time.Now().UnixNano(), rand.Int63())
}

// RequestContext middleware menambahkan trace ID dan timeout ke setiap request
func RequestContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Ambil atau buat trace ID
		traceID := c.GetHeader(TraceIDHeader)
		if traceID == "" {
			traceID = generateTraceID()
		}

		// Set trace ID ke context dan response header
		c.Set("trace_id", traceID)
		c.Header(TraceIDHeader, traceID)

		// Set timeout context
		ctx, cancel := context.WithTimeout(c.Request.Context(), defaultTimeout)
		defer cancel()

		// Replace request context
		c.Request = c.Request.WithContext(ctx)

		// Track waktu mulai untuk latency logging
		c.Set("start_time", time.Now())

		c.Next()
	}
}

// Timeout middleware untuk endpoint tertentu dengan custom timeout
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)

		// Channel untuk mendeteksi timeout
		done := make(chan struct{})
		go func() {
			c.Next()
			close(done)
		}()

		select {
		case <-done:
			// Request selesai normal
		case <-ctx.Done():
			c.JSON(http.StatusGatewayTimeout, gin.H{
				"error":    "request timeout",
				"message":  "Request took too long to process",
				"trace_id": c.GetString("trace_id"),
			})
			c.Abort()
		}
	}
}
