package main

import (
	"encoding/json"
	"log"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
)

const exchange = "driver.commands"

func mustGetenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if fallback != "" {
		return fallback
	}
	log.Fatalf("missing required env var %s", key)
	return ""
}

func main() {
	driverID := mustGetenv("DRIVER_ID", "")
	rabbitURL := mustGetenv("RABBITMQ_URL", "")

	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		log.Fatalf("failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open channel: %v", err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		log.Fatalf("failed to declare exchange: %v", err)
	}

	queueName := "driver-" + driverID + "-commands"
	routingKey := "driver." + driverID + ".command"

	q, err := ch.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		log.Fatalf("failed to declare queue: %v", err)
	}

	if err := ch.QueueBind(q.Name, routingKey, exchange, false, nil); err != nil {
		log.Fatalf("failed to bind queue: %v", err)
	}

	// autoAck=false: we ACK manually after "executing" the command, so a
	// crash mid-processing leaves the message for redelivery instead of
	// silently losing it.
	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("failed to start consuming: %v", err)
	}

	log.Printf("driver-simulator[%s] listening for commands on queue %q", driverID, q.Name)

	for msg := range msgs {
		var command struct {
			Type     string `json:"type"`
			IssuedAt string `json:"issuedAt"`
		}
		if err := json.Unmarshal(msg.Body, &command); err != nil {
			log.Printf("driver-simulator[%s] received malformed command, discarding: %v", driverID, err)
			msg.Nack(false, false)
			continue
		}

		log.Printf("driver-simulator[%s] executing command %q (issued %s)", driverID, command.Type, command.IssuedAt)
		msg.Ack(false)
	}
}
