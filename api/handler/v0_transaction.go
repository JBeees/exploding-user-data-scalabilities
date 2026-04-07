package handler

import (
	// "context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"peak-load-management/db"
)

// ─────────────────────────────────────────────────────────────────────────────
// V0 Handler — TANPA OPTIMASI (untuk perbandingan baseline)
// Tidak ada: Redis cache, RabbitMQ queue, circuit breaker
// Semua request langsung ke database secara synchronous
// ─────────────────────────────────────────────────────────────────────────────

// CreateTransactionV0 — POST /v0/transactions
// Write langsung ke DB secara sync, tanpa queue
func CreateTransactionV0(c *gin.Context) {
	traceID := c.GetString("trace_id")
	ctx := c.Request.Context()

	var req CreateTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "invalid request",
			"message":  err.Error(),
			"trace_id": traceID,
			"version":  "v0-no-optimization",
		})
		return
	}

	// Validasi user — langsung ke DB write (tidak pakai read replica)
	var userExists bool
	err := db.DB.Write.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND status = 'active')",
		req.UserID,
	).Scan(&userExists)

	if err != nil || !userExists {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":    "invalid user",
			"message":  "User not found or inactive",
			"trace_id": traceID,
			"version":  "v0-no-optimization",
		})
		return
	}

	txnID := uuid.New().String()
	refID := "REF-" + time.Now().Format("20060102") + "-" + txnID[:8]

	// ─── TANPA OPTIMASI: langsung proses semua secara sync ───────────────────
	// Tidak ada queue, tidak ada async processing
	// Semua dalam satu DB transaction yang blocking

	tx, err := db.DB.Write.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "failed to begin transaction",
			"trace_id": traceID,
			"version":  "v0-no-optimization",
		})
		return
	}
	defer tx.Rollback()

	// Insert transaksi
	_, err = tx.ExecContext(ctx,
		`INSERT INTO transactions (id, user_id, type, amount, status, reference_id, description)
		 VALUES ($1, $2, $3, $4, 'completed', $5, $6)`,
		txnID, req.UserID, req.Type, req.Amount, refID, req.Description,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "failed to insert transaction",
			"trace_id": traceID,
			"version":  "v0-no-optimization",
		})
		return
	}

	// Update balance langsung (tanpa queue, sync)
	var balanceQuery string
	switch req.Type {
	case "credit":
		balanceQuery = `UPDATE users SET balance = balance + $1 WHERE id = $2`
	case "debit", "transfer":
		balanceQuery = `UPDATE users SET balance = balance - $1 WHERE id = $2`
	}

	if balanceQuery != "" {
		_, err = tx.ExecContext(ctx, balanceQuery, req.Amount, req.UserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":    "failed to update balance",
				"trace_id": traceID,
				"version":  "v0-no-optimization",
			})
			return
		}
	}

	// Simulasi processing time tanpa optimasi (lebih lama)
	time.Sleep(100 * time.Millisecond)

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "failed to commit transaction",
			"trace_id": traceID,
			"version":  "v0-no-optimization",
		})
		return
	}

	// Return 200 (bukan 202) karena sync — user harus tunggu sampai selesai
	c.JSON(http.StatusOK, gin.H{
		"id":           txnID,
		"status":       "completed",
		"reference_id": refID,
		"message":      "Transaction processed synchronously",
		"trace_id":     traceID,
		"version":      "v0-no-optimization",
	})
}

// GetTransactionV0 — GET /v0/transactions/:id
// Langsung query DB, tanpa Redis cache
func GetTransactionV0(c *gin.Context) {
	traceID := c.GetString("trace_id")
	ctx := c.Request.Context()
	txnID := c.Param("id")

	if txnID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "missing transaction id",
			"trace_id": traceID,
			"version":  "v0-no-optimization",
		})
		return
	}

	// ─── TANPA OPTIMASI: langsung query DB setiap request ────────────────────
	// Tidak ada cache check, setiap request pasti hit database
	txn, err := queryTransaction(ctx, txnID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"error":    "transaction not found",
				"trace_id": traceID,
				"version":  "v0-no-optimization",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "failed to fetch transaction",
			"trace_id": traceID,
			"version":  "v0-no-optimization",
		})
		return
	}

	// Tidak ada X-Cache header karena tidak pakai cache
	c.Header("X-Cache", "DISABLED")
	c.Header("X-Version", "v0-no-optimization")
	c.JSON(http.StatusOK, txn)
}

// GetUserBalanceV0 — GET /v0/users/:id/balance
// Langsung query DB, tanpa Redis cache
func GetUserBalanceV0(c *gin.Context) {
	traceID := c.GetString("trace_id")
	ctx := c.Request.Context()
	userID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "missing user id",
			"trace_id": traceID,
			"version":  "v0-no-optimization",
		})
		return
	}

	// ─── TANPA OPTIMASI: langsung query DB setiap request ────────────────────
	// Tidak ada cache, setiap request pasti hit database
	resp, err := queryUserBalance(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"error":    "user not found",
				"trace_id": traceID,
				"version":  "v0-no-optimization",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "failed to fetch user balance",
			"trace_id": traceID,
			"version":  "v0-no-optimization",
		})
		return
	}

	resp.FetchedAt = time.Now()

	// Tidak ada X-Cache header
	c.Header("X-Cache", "DISABLED")
	c.Header("X-Version", "v0-no-optimization")
	c.JSON(http.StatusOK, resp)
}
