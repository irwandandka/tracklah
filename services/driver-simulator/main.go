package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"math/rand/v2"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	commandsExchange = "driver.commands"
	locationExchange = "location.events"
	pendingBacklog   = 100
)

// Roughly central Jakarta. Drivers start within startRadiusMeters of it.
var cityCenter = Point{Lat: -6.2000, Lng: 106.8000}

const startRadiusMeters = 2000.0

type config struct {
	driverIDs    []string
	rabbitURL    string
	apiURL       string
	pingInterval time.Duration
	speedMPS     float64
}

func loadConfig() (config, error) {
	var c config

	ids := os.Getenv("DRIVER_IDS")
	if ids == "" {
		ids = os.Getenv("DRIVER_ID") // single-driver setup from before DRIVER_IDS existed
	}
	for _, id := range strings.Split(ids, ",") {
		if id = strings.TrimSpace(id); id != "" {
			c.driverIDs = append(c.driverIDs, id)
		}
	}
	if len(c.driverIDs) == 0 {
		return c, fmt.Errorf("missing required env var DRIVER_IDS (comma-separated)")
	}

	if c.rabbitURL = os.Getenv("RABBITMQ_URL"); c.rabbitURL == "" {
		return c, fmt.Errorf("missing required env var RABBITMQ_URL")
	}
	if c.apiURL = strings.TrimRight(os.Getenv("API_URL"), "/"); c.apiURL == "" {
		return c, fmt.Errorf("missing required env var API_URL")
	}

	c.pingInterval = 10 * time.Second
	if v := os.Getenv("PING_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return c, fmt.Errorf("invalid PING_INTERVAL %q", v)
		}
		c.pingInterval = d
	}

	c.speedMPS = 12 // ~43 km/h, city traffic
	if v := os.Getenv("SPEED_MPS"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 {
			return c, fmt.Errorf("invalid SPEED_MPS %q", v)
		}
		c.speedMPS = f
	}
	return c, nil
}

// locationPublisher serialises publishes: every driver goroutine pings
// through one AMQP channel, and channels aren't safe for concurrent use.
type locationPublisher struct {
	mu sync.Mutex
	ch *amqp.Channel
}

func (p *locationPublisher) ping(driverID string, pt Point) error {
	payload, err := json.Marshal(map[string]any{
		"driverId":  driverID,
		"lat":       pt.Lat,
		"lng":       pt.Lng,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ch.Publish(locationExchange, "location."+driverID+".ping", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        payload,
	})
}

// consumeCommands is RabbitMQ Case 1 (direct exchange): each driver has its
// own command queue. Commands are only logged for now.
func consumeCommands(ctx context.Context, conn *amqp.Connection, driverID string) {
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("[%s] failed to open command channel: %v", driverID, err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(commandsExchange, "direct", true, false, false, false, nil); err != nil {
		log.Fatalf("failed to declare commands exchange: %v", err)
	}
	q, err := ch.QueueDeclare("driver-"+driverID+"-commands", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("[%s] failed to declare queue: %v", driverID, err)
	}
	if err := ch.QueueBind(q.Name, "driver."+driverID+".command", commandsExchange, false, nil); err != nil {
		log.Fatalf("[%s] failed to bind queue: %v", driverID, err)
	}
	// autoAck=false: ACK after "executing", so a crash mid-processing
	// leaves the message for redelivery instead of losing it.
	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("[%s] failed to start consuming: %v", driverID, err)
	}
	log.Printf("[%s] listening for commands on queue %q", driverID, q.Name)

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				return
			}
			var command struct {
				Type     string `json:"type"`
				IssuedAt string `json:"issuedAt"`
			}
			if err := json.Unmarshal(msg.Body, &command); err != nil {
				log.Printf("[%s] malformed command, discarding: %v", driverID, err)
				msg.Nack(false, false)
				continue
			}
			log.Printf("[%s] executing command %q (issued %s)", driverID, command.Type, command.IssuedAt)
			msg.Ack(false)
		}
	}
}

// seedFor gives each driver a stable random stream, so driver-1 starts in
// the same spot on every restart.
func seedFor(driverID string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(driverID))
	return h.Sum64()
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	conn, err := amqp.Dial(cfg.rabbitURL)
	if err != nil {
		log.Fatalf("failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	publishCh, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open publish channel: %v", err)
	}
	defer publishCh.Close()
	if err := publishCh.ExchangeDeclare(locationExchange, "topic", true, false, false, false, nil); err != nil {
		log.Fatalf("failed to declare location exchange: %v", err)
	}
	pub := &locationPublisher{ch: publishCh}

	dispatchCh, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open dispatch channel: %v", err)
	}
	defer dispatchCh.Close()

	api := newHTTPTripAPI(cfg.apiURL)
	pending := make(chan string, pendingBacklog)
	active := newActiveTrips()
	stepMeters := cfg.speedMPS * cfg.pingInterval.Seconds()

	var wg sync.WaitGroup
	for _, id := range cfg.driverIDs {
		seed := seedFor(id)
		rng := rand.New(rand.NewPCG(seed, seed>>1))
		start := randomPointNear(rng, cityCenter, startRadiusMeters)
		d := &driver{
			id: id, pos: start, home: start,
			api: api, ping: pub.ping,
			interval: cfg.pingInterval, stepMeters: stepMeters,
			rng: rng, active: active,
		}
		wg.Add(2)
		go func() { defer wg.Done(); d.run(ctx, pending) }()
		go func() { defer wg.Done(); consumeCommands(ctx, conn, id) }()
		log.Printf("[%s] online at %.5f,%.5f", id, start.Lat, start.Lng)
	}

	if err := consumeTripEvents(ctx, dispatchCh, pending, active); err != nil {
		log.Fatalf("dispatch: %v", err)
	}
	wg.Wait()
	log.Print("driver-simulator: shut down")
}
