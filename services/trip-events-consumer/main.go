package main

import (
	"encoding/json"
	"log"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
)

const exchange = "trip.lifecycle"

type tripEvent struct {
	TripID     string `json:"tripId"`
	Status     string `json:"status"`
	DriverID   string `json:"driverId"`
	OccurredAt string `json:"occurredAt"`
}

// consumeAs represents one independent downstream (analytics, notification,
// audit-log, ...). Each gets its OWN queue bound to the same fanout
// exchange, so every trip event is delivered to all of them - one consumer
// being slow or down doesn't affect the others.
func consumeAs(conn *amqp.Connection, label, queueName string) {
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("[%s] failed to open channel: %v", label, err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(exchange, "fanout", true, false, false, false, nil); err != nil {
		log.Fatalf("[%s] failed to declare exchange: %v", label, err)
	}

	q, err := ch.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		log.Fatalf("[%s] failed to declare queue: %v", label, err)
	}

	// Fanout ignores the routing key, so binding with "" attaches this
	// queue to every message published to the exchange.
	if err := ch.QueueBind(q.Name, "", exchange, false, nil); err != nil {
		log.Fatalf("[%s] failed to bind queue: %v", label, err)
	}

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("[%s] failed to start consuming: %v", label, err)
	}

	log.Printf("[%s] listening on queue %q", label, q.Name)

	for msg := range msgs {
		var event tripEvent
		if err := json.Unmarshal(msg.Body, &event); err != nil {
			log.Printf("[%s] received malformed event, discarding: %v", label, err)
			msg.Nack(false, false)
			continue
		}

		log.Printf("[%s] trip=%s status=%s driverId=%s", label, event.TripID, event.Status, event.DriverID)
		msg.Ack(false)
	}
}

func main() {
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		log.Fatal("missing required env var RABBITMQ_URL")
	}

	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		log.Fatalf("failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	go consumeAs(conn, "analytics", "analytics-consumer")
	go consumeAs(conn, "notification", "notification-consumer")
	go consumeAs(conn, "audit-log", "audit-log-consumer")

	select {} // block forever - all the work happens in the goroutines above
}
