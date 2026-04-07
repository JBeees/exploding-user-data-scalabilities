package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

var Client *redis.Client

func Init() {
	Client = redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", os.Getenv("REDIS_HOST"), os.Getenv("REDIS_PORT")),
		Password:     os.Getenv("REDIS_PASSWORD"),
		DB:           0,
		PoolSize:     20,
		MinIdleConns: 5,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := Client.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Failed to connect Redis: %v", err)
	}

	log.Println("✅ Redis connected")
}

func DefaultTTL() time.Duration {
	ttl, _ := strconv.Atoi(getEnv("CACHE_TTL_SECONDS", "30"))
	return time.Duration(ttl) * time.Second
}

// Set menyimpan value ke Redis dengan TTL
func Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache marshal error: %w", err)
	}
	return Client.Set(ctx, key, data, ttl).Err()
}

// Get mengambil value dari Redis dan unmarshal ke dest
func Get(ctx context.Context, key string, dest interface{}) (bool, error) {
	data, err := Client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil // cache miss
	}
	if err != nil {
		return false, fmt.Errorf("cache get error: %w", err)
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return false, fmt.Errorf("cache unmarshal error: %w", err)
	}
	return true, nil
}

// Delete menghapus key dari Redis
func Delete(ctx context.Context, key string) error {
	return Client.Del(ctx, key).Err()
}

// TransactionKey returns cache key for a transaction
func TransactionKey(id string) string {
	return fmt.Sprintf("txn:%s", id)
}

// UserBalanceKey returns cache key for user balance
func UserBalanceKey(id string) string {
	return fmt.Sprintf("user:balance:%s", id)
}

func Close() {
	if Client != nil {
		Client.Close()
		log.Println("🔌 Redis connection closed")
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
