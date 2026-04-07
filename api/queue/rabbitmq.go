package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

var (
	conn    *amqp.Connection
	channel *amqp.Channel
)

type TransactionMessage struct {
	TransactionID string          `json:"transaction_id"`
	UserID        string          `json:"user_id"`
	Type          string          `json:"type"`
	Amount        float64         `json:"amount"`
	Description   string          `json:"description"`
	TraceID       string          `json:"trace_id"`
	CreatedAt     time.Time       `json:"created_at"`
}

func Init() {
	url := os.Getenv("RABBITMQ_URL")

	var err error
	// Retry connection (RabbitMQ might take a moment)
	for i := 0; i < 5; i++ {
		conn, err = amqp.Dial(url)
		if err == nil {
			break
		}
		log.Printf("⏳ RabbitMQ not ready, retrying (%d/5)...", i+1)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatalf("❌ Failed to connect RabbitMQ: %v", err)
	}

	channel, err = conn.Channel()
	if err != nil {
		log.Fatalf("❌ Failed to open RabbitMQ channel: %v", err)
	}

	// Declare queue (idempotent)
	queueName := getEnv("RABBITMQ_QUEUE_TRANSACTION", "transactions_queue")
	_, err = channel.QueueDeclare(
		queueName,
		true,  // durable - survive restarts
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		amqp.Table{
			"x-max-length":  100000,        // max 100k messages
			"x-overflow":    "reject-publish", // backpressure: reject when full
		},
	)
	if err != nil {
		log.Fatalf("❌ Failed to declare queue: %v", err)
	}

	// Prefetch limit (backpressure on consumer side)
	prefetch, _ := strconv.Atoi(getEnv("RABBITMQ_PREFETCH_COUNT", "10"))
	channel.Qos(prefetch, 0, false)

	log.Println("✅ RabbitMQ connected, queue declared")
}

// Publish mengirim pesan ke queue (async write path)
func Publish(ctx context.Context, msg TransactionMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("queue marshal error: %w", err)
	}

	queueName := getEnv("RABBITMQ_QUEUE_TRANSACTION", "transactions_queue")

	err = channel.PublishWithContext(ctx,
		"",        // exchange (default)
		queueName, // routing key
		true,      // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // survive broker restart
			MessageId:    msg.TransactionID,
			Timestamp:    time.Now(),
			Body:         data,
		},
	)
	if err != nil {
		return fmt.Errorf("queue publish error: %w", err)
	}

	return nil
}

// QueueDepth returns current number of pending messages
func QueueDepth() (int, error) {
	queueName := getEnv("RABBITMQ_QUEUE_TRANSACTION", "transactions_queue")
	q, err := channel.QueueInspect(queueName)
	if err != nil {
		return 0, err
	}
	return q.Messages, nil
}

// StartConsumer memproses pesan dari queue secara async
func StartConsumer(processFunc func(msg TransactionMessage) error) {
	queueName := getEnv("RABBITMQ_QUEUE_TRANSACTION", "transactions_queue")

	msgs, err := channel.Consume(
		queueName,
		"",    // consumer tag
		false, // auto-ack (manual ack for reliability)
		false, false, false, nil,
	)
	if err != nil {
		log.Fatalf("❌ Failed to start consumer: %v", err)
	}

	log.Println("👂 Queue consumer started")

	go func() {
		for d := range msgs {
			var msg TransactionMessage
			if err := json.Unmarshal(d.Body, &msg); err != nil {
				log.Printf("❌ Failed to unmarshal queue message: %v", err)
				d.Nack(false, false) // discard malformed message
				continue
			}

			if err := processFunc(msg); err != nil {
				log.Printf("❌ Failed to process transaction %s: %v", msg.TransactionID, err)
				d.Nack(false, true) // requeue on error
			} else {
				d.Ack(false)
			}
		}
	}()
}

func Close() {
	if channel != nil {
		channel.Close()
	}
	if conn != nil {
		conn.Close()
		log.Println("🔌 RabbitMQ connection closed")
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
