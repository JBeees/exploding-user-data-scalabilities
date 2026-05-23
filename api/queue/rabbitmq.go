package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
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
	url := strings.TrimSpace(os.Getenv("RABBITMQ_URL"))

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

	// ─── Dead Letter Queue (DLQ) Setup ───────────────────────────────────
	dlxName := "transactions_dlx"
	dlqName := "transactions_dlq"
	dlqRoutingKey := "transactions_dlq_routing_key"

	// Declare Dead Letter Exchange (DLX)
	err = channel.ExchangeDeclare(
		dlxName,
		"direct",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	)
	if err != nil {
		log.Fatalf("❌ Failed to declare DLX: %v", err)
	}

	// Declare Dead Letter Queue (DLQ)
	_, err = channel.QueueDeclare(
		dlqName,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,
	)
	if err != nil {
		log.Fatalf("❌ Failed to declare DLQ: %v", err)
	}

	// Bind DLQ to DLX
	err = channel.QueueBind(
		dlqName,
		dlqRoutingKey,
		dlxName,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("❌ Failed to bind DLQ to DLX: %v", err)
	}

	// Declare main queue with DLX bindings
	queueName := getEnv("RABBITMQ_QUEUE_TRANSACTION", "transactions_queue")
	_, err = channel.QueueDeclare(
		queueName,
		true,  // durable - survive restarts
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		amqp.Table{
			"x-max-length":             100000,           // max 100k messages
			"x-overflow":               "reject-publish", // backpressure: reject when full
			"x-dead-letter-exchange":    dlxName,
			"x-dead-letter-routing-key": dlqRoutingKey,
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
				retryOrDlq(d, msg, err)
			} else {
				d.Ack(false)
			}
		}
	}()
}

// retryOrDlq menangani retry pesan yang gagal hingga 3 kali dengan backoff sebelum dikirim ke DLQ.
func retryOrDlq(d amqp.Delivery, msg TransactionMessage, processErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var retryCount int32 = 0
	if val, ok := d.Headers["x-retry-count"]; ok {
		if count, ok := val.(int32); ok {
			retryCount = count
		} else if count, ok := val.(int64); ok {
			retryCount = int32(count)
		}
	}

	if retryCount < 3 {
		retryCount++
		log.Printf("⚠️ Retrying transaction %s (%d/3) in %ds due to: %v", msg.TransactionID, retryCount, retryCount*retryCount, processErr)
		
		// Backoff delay sebelum mempublikasikan ulang (1s, 4s, 9s)
		time.Sleep(time.Duration(retryCount*retryCount) * time.Second)

		headers := amqp.Table{
			"x-retry-count": retryCount,
		}
		
		data, _ := json.Marshal(msg)
		err := channel.PublishWithContext(ctx,
			"",           // exchange
			d.RoutingKey, // queue name
			true,         // mandatory
			false,        // immediate
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				MessageId:    msg.TransactionID,
				Timestamp:    time.Now(),
				Headers:      headers,
				Body:         data,
			},
		)
		if err != nil {
			log.Printf("❌ Failed to republish for retry: %v", err)
			d.Nack(false, true) // fallback ke standard requeue jika publish ulang gagal
			return
		}
		
		d.Ack(false) // Acknowledge pesan lama
	} else {
		log.Printf("❌ Transaction %s failed after 3 retries. Sending to DLQ. Error: %v", msg.TransactionID, processErr)
		// Nack dengan requeue=false akan memicu RabbitMQ memindahkan pesan ke DLQ
		d.Nack(false, false)
	}
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
		return strings.TrimSpace(v)
	}
	return def
}
