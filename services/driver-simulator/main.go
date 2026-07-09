package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	commandsExchange = "driver.commands"
	locationExchange = "location.events"
)

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

// publishLocationPings simulates a driver's GPS sending a ping every few
// seconds, wandering slightly from a starting point each tick.
func publishLocationPings(ch *amqp.Channel, driverID string) {
	lat, lng := -6.2000, 106.8000 // roughly Jakarta
	routingKey := "location." + driverID + ".ping"

	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		lat += (rand.Float64() - 0.5) * 0.002
		lng += (rand.Float64() - 0.5) * 0.002

		payload, _ := json.Marshal(map[string]any{
			"driverId":  driverID,
			"lat":       lat,
			"lng":       lng,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})

		err := ch.Publish(locationExchange, routingKey, false, false, amqp.Publishing{
			ContentType: "application/json",
			Body:        payload,
		})
		if err != nil {
			log.Printf("driver-simulator: failed to publish location ping: %v", err)
			continue
		}
		log.Printf("driver-simulator: published location ping lat=%f lng=%f", lat, lng)
	}
}

func consumeCommands(ch *amqp.Channel, driverID string) {
	queueName := "driver-" + driverID + "-commands"
	routingKey := "driver." + driverID + ".command"

	q, err := ch.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		log.Fatalf("failed to declare queue: %v", err)
	}

	if err := ch.QueueBind(q.Name, routingKey, commandsExchange, false, nil); err != nil {
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

func main() {
	driverID := mustGetenv("DRIVER_ID", "")
	rabbitURL := mustGetenv("RABBITMQ_URL", "")

	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		log.Fatalf("failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	// Separate channels for publishing (location pings) and consuming
	// (commands) so the two concerns don't share mutable AMQP state.
	publishCh, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open publish channel: %v", err)
	}
	defer publishCh.Close()

	consumeCh, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open consume channel: %v", err)
	}
	defer consumeCh.Close()

	if err := consumeCh.ExchangeDeclare(commandsExchange, "direct", true, false, false, false, nil); err != nil {
		log.Fatalf("failed to declare commands exchange: %v", err)
	}
	if err := publishCh.ExchangeDeclare(locationExchange, "topic", true, false, false, false, nil); err != nil {
		log.Fatalf("failed to declare location exchange: %v", err)
	}

	go publishLocationPings(publishCh, driverID)

	consumeCommands(consumeCh, driverID)
}
