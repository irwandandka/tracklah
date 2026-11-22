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
	driverIDs []string
	rabbitURL string
	apiURL    string
	// metricsAddr is where Prometheus scrapes /metrics (METRICS_ADDR).
	metricsAddr string
	idlePing    time.Duration
	tripPing    time.Duration
	speedMPS    float64
	shifts      shiftPlan
}

// parseDurationRange reads "20m-40m" (or a single "30m") into a range.
func parseDurationRange(v string) (durationRange, error) {
	lo, hi, found := strings.Cut(v, "-")
	if !found {
		hi = lo
	}
	min, err1 := time.ParseDuration(strings.TrimSpace(lo))
	max, err2 := time.ParseDuration(strings.TrimSpace(hi))
	if err1 != nil || err2 != nil || min <= 0 || max < min {
		return durationRange{}, fmt.Errorf("invalid duration range %q, want e.g. 20m-40m", v)
	}
	return durationRange{Min: min, Max: max}, nil
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
	if c.metricsAddr = os.Getenv("METRICS_ADDR"); c.metricsAddr == "" {
		c.metricsAddr = ":9464"
	}

	// Waiting for an order: 20s keeps a driver inside the app's
	// "updatedAt > now-30s" online window with one ping of slack.
	// On a trip: 10s, so the rider sees the driver move more smoothly.
	c.idlePing, c.tripPing = 20*time.Second, 10*time.Second
	for env, dst := range map[string]*time.Duration{
		"IDLE_PING_INTERVAL": &c.idlePing,
		"TRIP_PING_INTERVAL": &c.tripPing,
	} {
		if v := os.Getenv(env); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil || d <= 0 {
				return c, fmt.Errorf("invalid %s %q", env, v)
			}
			*dst = d
		}
	}

	c.speedMPS = 12 // ~43 km/h, city traffic
	if v := os.Getenv("SPEED_MPS"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 {
			return c, fmt.Errorf("invalid SPEED_MPS %q", v)
		}
		c.speedMPS = f
	}

	// Online/offline shifts. ALWAYS_ONLINE=true turns them off, which is
	// handy when testing trips locally.
	if os.Getenv("ALWAYS_ONLINE") != "true" {
		c.shifts = shiftPlan{
			online:  durationRange{20 * time.Minute, 40 * time.Minute},
			offline: durationRange{10 * time.Minute, 30 * time.Minute},
		}
		for env, r := range map[string]*durationRange{
			"ONLINE_DURATION":  &c.shifts.online,
			"OFFLINE_DURATION": &c.shifts.offline,
		} {
			if v := os.Getenv(env); v != "" {
				parsed, err := parseDurationRange(v)
				if err != nil {
					return c, fmt.Errorf("%s: %w", env, err)
				}
				*r = parsed
			}
		}
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
	watchPending(pending)
	serveMetrics(cfg.metricsAddr)

	var wg sync.WaitGroup
	for _, id := range cfg.driverIDs {
		seed := seedFor(id)
		rng := rand.New(rand.NewPCG(seed, seed>>1))
		start := randomPointNear(rng, cityCenter, startRadiusMeters)
		d := &driver{
			id: id, pos: start, home: start,
			api: api, ping: pub.ping,
			idle:   motion{cfg.idlePing, cfg.speedMPS * cfg.idlePing.Seconds()},
			onTrip: motion{cfg.tripPing, cfg.speedMPS * cfg.tripPing.Seconds()},
			rng:    rng, active: active,
			shifts: cfg.shifts,
		}
		wg.Add(2)
		go func() { defer wg.Done(); d.run(ctx, pending) }()
		go func() { defer wg.Done(); consumeCommands(ctx, conn, id) }()
		log.Printf("[%s] starting at %.5f,%.5f", id, start.Lat, start.Lng)
	}

	if err := consumeTripEvents(ctx, dispatchCh, pending, active); err != nil {
		log.Fatalf("dispatch: %v", err)
	}
	wg.Wait()
	log.Print("driver-simulator: shut down")
}
