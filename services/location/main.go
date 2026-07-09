package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
)

const locationExchange = "location.events"

type LocationPing struct {
	DriverID string  `json:"driverId"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var ping LocationPing
	if err := json.NewDecoder(r.Body).Decode(&ping); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if ping.DriverID == "" {
		http.Error(w, "driverId is required", http.StatusBadRequest)
		return
	}

	log.Printf("location ping (HTTP): driverId=%s lat=%f lng=%f", ping.DriverID, ping.Lat, ping.Lng)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "received"})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// consumeLocationEvents listens for pings from ANY driver via a wildcard
// binding, instead of the driver-specific queues used for commands. This
// is the point where RabbitMQ's Topic exchange earns its keep over Direct:
// one queue can receive events from every driver without declaring a
// queue per driver.
func consumeLocationEvents(ch *amqp.Channel) {
	if err := ch.ExchangeDeclare(locationExchange, "topic", true, false, false, false, nil); err != nil {
		log.Fatalf("failed to declare location exchange: %v", err)
	}

	q, err := ch.QueueDeclare("location-realtime", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("failed to declare queue: %v", err)
	}

	// "location.*.ping" - the * wildcard matches exactly one word, so this
	// binding catches location.<any-driver-id>.ping regardless of which
	// driver sent it.
	if err := ch.QueueBind(q.Name, "location.*.ping", locationExchange, false, nil); err != nil {
		log.Fatalf("failed to bind queue: %v", err)
	}

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("failed to start consuming: %v", err)
	}

	log.Printf("location service: listening for location pings on queue %q", q.Name)

	for msg := range msgs {
		var ping LocationPing
		if err := json.Unmarshal(msg.Body, &ping); err != nil {
			log.Printf("location service: received malformed ping, discarding: %v", err)
			msg.Nack(false, false)
			continue
		}

		log.Printf("location ping (RabbitMQ): driverId=%s lat=%f lng=%f", ping.DriverID, ping.Lat, ping.Lng)
		msg.Ack(false)
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		log.Fatal("missing required env var RABBITMQ_URL")
	}

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

	go consumeLocationEvents(ch)

	http.HandleFunc("/ping", pingHandler)
	http.HandleFunc("/health", healthHandler)

	log.Printf("location service listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
