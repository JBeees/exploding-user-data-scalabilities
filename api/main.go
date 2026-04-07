package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"peak-load-management/cache"
	"peak-load-management/db"
	"peak-load-management/handler"
	"peak-load-management/metrics"
	"peak-load-management/middleware"
	"peak-load-management/queue"
)

func main() {
	// ─── Init semua dependencies ────────────────────────────────────────────
	database := db.Init()
	defer db.Close()

	cache.Init()
	defer cache.Close()

	queue.Init()
	defer queue.Close()

	middleware.InitCircuitBreakers()

	// ─── Start async transaction consumer ───────────────────────────────────
	queue.StartConsumer(func(msg queue.TransactionMessage) error {
		return processTransaction(database, msg)
	})

	// ─── Setup Gin router ───────────────────────────────────────────────────
	if os.Getenv("ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// Global middleware
	r.Use(middleware.RequestContext()) // trace ID + timeout
	r.Use(middleware.Logger())         // structured JSON logging
	r.Use(middleware.RateLimit())      // token bucket rate limiter
	r.Use(gin.Recovery())              // recover from panics

	// ─── Routes ─────────────────────────────────────────────────────────────

	// Health check (no rate limit, no circuit breaker)
	r.GET("/health", healthCheck)

	// Prometheus metrics endpoint
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API routes — dengan circuit breaker
	api := r.Group("/")
	api.Use(middleware.CircuitBreakerMiddleware(middleware.GetDBCircuitBreaker()))
	{
		// Write-heavy: async via queue
		api.POST("/transactions", handler.CreateTransaction)

		// Read-heavy: cached via Redis
		api.GET("/transactions/:id", handler.GetTransaction)
		api.GET("/users/:id/balance", handler.GetUserBalance)
	}

	// ─── V0 Routes — TANPA optimasi (untuk perbandingan baseline) ─────────
	// Tidak ada: Redis cache, RabbitMQ queue, circuit breaker, rate limit
	v0 := r.Group("/v0")
	{
		// Write sync langsung ke DB (tanpa queue)
		v0.POST("/transactions", handler.CreateTransactionV0)

		// Read langsung ke DB (tanpa Redis cache)
		v0.GET("/transactions/:id", handler.GetTransactionV0)
		v0.GET("/users/:id/balance", handler.GetUserBalanceV0)
	}


	// ─── Start server dengan graceful shutdown ───────────────────────────────
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	// Start server di goroutine
	go func() {
		log.Printf("🚀 Server started on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Server error: %v", err)
		}
	}()

	// Start background metrics collector
	go collectMetrics(database)

	// Tunggu signal shutdown (Ctrl+C atau SIGTERM dari Docker)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("⏳ Shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("❌ Forced shutdown: %v", err)
	}

	log.Println("✅ Server stopped")
}

// ─── Health Check ────────────────────────────────────────────────────────────

func healthCheck(c *gin.Context) {
	dbStatus := "ok"
	if err := db.DB.Write.Ping(); err != nil {
		dbStatus = "error: " + err.Error()
	}

	cacheStatus := "ok"
	if err := cache.Client.Ping(c.Request.Context()).Err(); err != nil {
		cacheStatus = "error: " + err.Error()
	}

	queueDepth, _ := queue.QueueDepth()

	status := http.StatusOK
	if dbStatus != "ok" || cacheStatus != "ok" {
		status = http.StatusServiceUnavailable
	}

	c.JSON(status, gin.H{
		"status":      "ok",
		"timestamp":   time.Now().UTC(),
		"trace_id":    c.GetString("trace_id"),
		"db_write":    dbStatus,
		"cache":       cacheStatus,
		"queue_depth": queueDepth,
	})
}

// ─── Queue Consumer: proses transaksi async ──────────────────────────────────

func processTransaction(database *db.Database, msg queue.TransactionMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Update status → 'processing'
	_, err := database.Write.ExecContext(ctx,
		`UPDATE transactions SET status = 'processing' WHERE id = $1`,
		msg.TransactionID,
	)
	if err != nil {
		return err
	}

	// Simulasi proses bisnis (validasi, update balance, dll)
	time.Sleep(50 * time.Millisecond) // simulasi processing time

	// Update status → 'completed' + update user balance
	tx, err := database.Write.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update transaction status
	_, err = tx.ExecContext(ctx,
		`UPDATE transactions SET status = 'completed' WHERE id = $1`,
		msg.TransactionID,
	)
	if err != nil {
		return err
	}

	// Update user balance berdasarkan type transaksi
	var balanceQuery string
	switch msg.Type {
	case "credit":
		balanceQuery = `UPDATE users SET balance = balance + $1 WHERE id = $2`
	case "debit", "transfer":
		balanceQuery = `UPDATE users SET balance = balance - $1 WHERE id = $2`
	}

	if balanceQuery != "" {
		_, err = tx.ExecContext(ctx, balanceQuery, msg.Amount, msg.UserID)
		if err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Invalidate cache setelah update
	cache.Delete(ctx, cache.UserBalanceKey(msg.UserID))
	cache.Delete(ctx, cache.TransactionKey(msg.TransactionID))

	metrics.QueueProcessedTotal.WithLabelValues("success").Inc()
	metrics.TransactionsCreatedTotal.WithLabelValues(msg.Type, "completed").Inc()

	log.Printf(`{"event":"transaction_processed","id":"%s","type":"%s","amount":%.2f,"trace_id":"%s"}`,
		msg.TransactionID, msg.Type, msg.Amount, msg.TraceID)

	return nil
}

// ─── Background: update DB connection metrics ────────────────────────────────

func collectMetrics(database *db.Database) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		writeStats := database.Write.Stats()
		readStats := database.Read.Stats()

		metrics.DbConnectionsActive.WithLabelValues("write").Set(float64(writeStats.InUse))
		metrics.DbConnectionsActive.WithLabelValues("read").Set(float64(readStats.InUse))

		depth, err := queue.QueueDepth()
		if err == nil {
			metrics.QueueMessagesPending.Set(float64(depth))
		}
	}
}
