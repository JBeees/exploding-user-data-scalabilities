package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	_ "github.com/lib/pq"
)

// Database holds write and read connections
type Database struct {
	Write *sql.DB
	Read  *sql.DB
}

var DB *Database

func Init() *Database {
	writeDB := connect(os.Getenv("DB_WRITE_HOST"), os.Getenv("DB_WRITE_PORT"))
	readDB := connect(os.Getenv("DB_READ_HOST"), os.Getenv("DB_READ_PORT"))

	DB = &Database{
		Write: writeDB,
		Read:  readDB,
	}

	log.Println("✅ Database connected (write + read replica)")
	return DB
}

func connect(host, port string) *sql.DB {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port,
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
	)

	database, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("❌ Failed to open DB (%s:%s): %v", host, port, err)
	}

	maxOpen, _ := strconv.Atoi(getEnv("DB_MAX_OPEN_CONNS", "25"))
	maxIdle, _ := strconv.Atoi(getEnv("DB_MAX_IDLE_CONNS", "10"))

	database.SetMaxOpenConns(maxOpen)
	database.SetMaxIdleConns(maxIdle)
	database.SetConnMaxLifetime(5 * time.Minute)
	database.SetConnMaxIdleTime(2 * time.Minute)

	if err := database.Ping(); err != nil {
		log.Fatalf("❌ Failed to ping DB (%s:%s): %v", host, port, err)
	}

	log.Printf("✅ DB connected: %s:%s", host, port)
	return database
}

func Close() {
	if DB != nil {
		DB.Write.Close()
		DB.Read.Close()
		log.Println("🔌 Database connections closed")
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
