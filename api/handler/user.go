package handler

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"peak-load-management/cache"
	"peak-load-management/db"
	"peak-load-management/metrics"
	"peak-load-management/middleware"
)

// ─── Models ─────────────────────────────────────────────────────────────────

type UserBalanceResponse struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Balance   float64   `json:"balance"`
	Status    string    `json:"status"`
	FetchedAt time.Time `json:"fetched_at"`
}

// ─── Handlers ────────────────────────────────────────────────────────────────

// GetUserBalance godoc
// GET /users/:id/balance
// Read-heavy endpoint — di-cache Redis
func GetUserBalance(c *gin.Context) {
	traceID := c.GetString("trace_id")
	ctx := c.Request.Context()
	userID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "missing user id",
			"trace_id": traceID,
		})
		return
	}

	cacheKey := cache.UserBalanceKey(userID)

	// 1. Cek cache dulu
	var resp UserBalanceResponse
	hit, err := cache.Get(ctx, cacheKey, &resp)
	if err != nil {
		metrics.CacheMissesTotal.WithLabelValues("user_balance").Inc()
	} else if hit {
		metrics.CacheHitsTotal.WithLabelValues("user_balance").Inc()
		c.Header("X-Cache", "HIT")
		c.JSON(http.StatusOK, resp)
		return
	}

	metrics.CacheMissesTotal.WithLabelValues("user_balance").Inc()

	// 2. Query dari read replica via circuit breaker
	result, err := middleware.ExecuteWithCB(
		middleware.GetDBCircuitBreaker(),
		func() (interface{}, error) {
			return queryUserBalance(ctx, userID)
		},
	)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"error":    "user not found",
				"trace_id": traceID,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "failed to fetch user balance",
			"trace_id": traceID,
		})
		return
	}

	resp = result.(UserBalanceResponse)
	resp.FetchedAt = time.Now()

	// 3. Simpan ke cache
	cache.Set(ctx, cacheKey, resp, cache.DefaultTTL())

	c.Header("X-Cache", "MISS")
	c.JSON(http.StatusOK, resp)
}

// ─── Helper ──────────────────────────────────────────────────────────────────

func queryUserBalance(ctx context.Context, id string) (UserBalanceResponse, error) {
	var resp UserBalanceResponse

	err := db.DB.Read.QueryRowContext(ctx,
		`SELECT id, username, balance, status FROM users WHERE id = $1`,
		id,
	).Scan(&resp.UserID, &resp.Username, &resp.Balance, &resp.Status)

	return resp, err
}
