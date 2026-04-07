package handler

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"peak-load-management/cache"
	"peak-load-management/db"
	"peak-load-management/metrics"
	"peak-load-management/middleware"
	"peak-load-management/queue"
)

// ─── Models ─────────────────────────────────────────────────────────────────

type CreateTransactionRequest struct {
	UserID      string  `json:"user_id" binding:"required"`
	Type        string  `json:"type" binding:"required,oneof=credit debit transfer"`
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	Description string  `json:"description"`
}

type TransactionResponse struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Type        string    `json:"type"`
	Amount      float64   `json:"amount"`
	Status      string    `json:"status"`
	ReferenceID string    `json:"reference_id"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ─── Handlers ────────────────────────────────────────────────────────────────

// CreateTransaction godoc
// POST /transactions
// Write-heavy endpoint — diproses async via RabbitMQ
func CreateTransaction(c *gin.Context) {
	traceID := c.GetString("trace_id")
	ctx := c.Request.Context()

	var req CreateTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "invalid request",
			"message":  err.Error(),
			"trace_id": traceID,
		})
		return 	
	}

	// Validasi user exists (menggunakan read replica)
	var userExists bool
	err := db.DB.Read.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND status = 'active')",
		req.UserID,
	).Scan(&userExists)

	if err != nil || !userExists {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":    "invalid user",
			"message":  "User not found or inactive",
			"trace_id": traceID,
		})
		return
	}

	// Generate transaction ID dan reference
	txnID := uuid.New().String()
	refID := "REF-" + time.Now().Format("20060102") + "-" + txnID[:8]

	// Insert ke DB dengan status 'pending' (write → primary)
	_, err = middleware.ExecuteWithCB(
		middleware.GetDBCircuitBreaker(),
		func() (interface{}, error) {
			_, err := db.DB.Write.ExecContext(ctx,
				`INSERT INTO transactions (id, user_id, type, amount, status, reference_id, description)
				 VALUES ($1, $2, $3, $4, 'pending', $5, $6)`,
				txnID, req.UserID, req.Type, req.Amount, refID, req.Description,
			)
			return nil, err
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "failed to create transaction",
			"message":  err.Error(),
			"trace_id": traceID,
		})
		return
	}

	// Kirim ke queue untuk async processing
	msg := queue.TransactionMessage{
		TransactionID: txnID,
		UserID:        req.UserID,
		Type:          req.Type,
		Amount:        req.Amount,
		Description:   req.Description,
		TraceID:       traceID,
		CreatedAt:     time.Now(),
	}

	if err := queue.Publish(ctx, msg); err != nil {
		// Queue gagal tapi transaksi sudah masuk DB (status pending)
		c.JSON(http.StatusAccepted, gin.H{
			"id":           txnID,
			"status":       "pending",
			"reference_id": refID,
			"message":      "Transaction created, queued for processing",
			"trace_id":     traceID,
			"warning":      "Queue unavailable, manual processing required",
		})
		return
	}

	metrics.QueuePublishedTotal.Inc()
	metrics.TransactionsCreatedTotal.WithLabelValues(req.Type, "pending").Inc()

	c.JSON(http.StatusAccepted, gin.H{
		"id":           txnID,
		"status":       "pending",
		"reference_id": refID,
		"message":      "Transaction accepted and queued for processing",
		"trace_id":     traceID,
	})
}

// GetTransaction godoc
// GET /transactions/:id
// Read-heavy endpoint — di-cache Redis
func GetTransaction(c *gin.Context) {
	traceID := c.GetString("trace_id")
	ctx := c.Request.Context()
	txnID := c.Param("id")

	if txnID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "missing transaction id",
			"trace_id": traceID,
		})
		return
	}

	cacheKey := cache.TransactionKey(txnID)

	// 1. Cek cache dulu
	var txn TransactionResponse
	hit, err := cache.Get(ctx, cacheKey, &txn)
	if err != nil {
		// Cache error tidak fatal — fallback ke DB
		metrics.CacheMissesTotal.WithLabelValues("transaction").Inc()
	} else if hit {
		metrics.CacheHitsTotal.WithLabelValues("transaction").Inc()
		c.Header("X-Cache", "HIT")
		c.JSON(http.StatusOK, txn)
		return
	}

	metrics.CacheMissesTotal.WithLabelValues("transaction").Inc()

	// 2. Query dari read replica
	result, err := middleware.ExecuteWithCB(
		middleware.GetDBCircuitBreaker(),
		func() (interface{}, error) {
			return queryTransaction(ctx, txnID)
		},
	)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"error":    "transaction not found",
				"trace_id": traceID,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "failed to fetch transaction",
			"trace_id": traceID,
		})
		return
	}

	txn = result.(TransactionResponse)

	// 3. Simpan ke cache
	cache.Set(ctx, cacheKey, txn, cache.DefaultTTL())

	c.Header("X-Cache", "MISS")
	c.JSON(http.StatusOK, txn)
}

// ─── Helper ──────────────────────────────────────────────────────────────────

func queryTransaction(ctx context.Context, id string) (TransactionResponse, error) {
	var txn TransactionResponse
	var refID, desc sql.NullString

	err := db.DB.Read.QueryRowContext(ctx,
		`SELECT id, user_id, type, amount, status, reference_id, description, created_at, updated_at
		 FROM transactions WHERE id = $1`,
		id,
	).Scan(
		&txn.ID, &txn.UserID, &txn.Type, &txn.Amount,
		&txn.Status, &refID, &desc,
		&txn.CreatedAt, &txn.UpdatedAt,
	)
	if err != nil {
		return txn, err
	}

	txn.ReferenceID = refID.String
	txn.Description = desc.String
	return txn, nil
}
