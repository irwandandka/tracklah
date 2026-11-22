package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	tripLifecycleExchange = "trip.lifecycle"
	// One durable queue for the whole simulator, so "requested" events
	// published while it's down are still there when it comes back.
	dispatchQueue = "driver-simulator-dispatch"
)

var errCancelledByRider = errors.New("trip cancelled by rider")

type tripEvent struct {
	TripID   string `json:"tripId"`
	Status   string `json:"status"`
	DriverID string `json:"driverId"`
}

// activeTrips maps a trip a driver is currently heading to -> how to stop
// that driver, so a "cancelled" event can reach the right goroutine.
type activeTrips struct {
	mu sync.Mutex
	m  map[string]context.CancelCauseFunc
}

func newActiveTrips() *activeTrips {
	return &activeTrips{m: map[string]context.CancelCauseFunc{}}
}

func (a *activeTrips) add(tripID string, cancel context.CancelCauseFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.m[tripID] = cancel
}

func (a *activeTrips) remove(tripID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.m, tripID)
}

func (a *activeTrips) cancel(tripID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	stop, ok := a.m[tripID]
	if ok {
		stop(errCancelledByRider)
	}
	return ok
}

// handleEvent decides what a lifecycle event means for the simulator.
// Everything else (including the simulator's own assigned/in_progress/
// completed updates echoing back) is ignored.
func handleEvent(ev tripEvent, pending chan<- string, active *activeTrips) {
	switch ev.Status {
	case "requested":
		select {
		case pending <- ev.TripID:
		default:
			dispatchDropped.Inc()
			log.Printf("dispatch: backlog full, dropping trip %s", ev.TripID)
		}
	case "cancelled":
		if active.cancel(ev.TripID) {
			log.Printf("dispatch: trip %s cancelled by rider, driver released", ev.TripID)
		}
	}
}

func consumeTripEvents(ctx context.Context, ch *amqp.Channel, pending chan<- string, active *activeTrips) error {
	if err := ch.ExchangeDeclare(tripLifecycleExchange, "fanout", true, false, false, false, nil); err != nil {
		return err
	}
	q, err := ch.QueueDeclare(dispatchQueue, true, false, false, false, nil)
	if err != nil {
		return err
	}
	if err := ch.QueueBind(q.Name, "", tripLifecycleExchange, false, nil); err != nil {
		return err
	}
	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	log.Printf("dispatch: listening for trip events on queue %q", q.Name)

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return errors.New("trip events channel closed")
			}
			var ev tripEvent
			if err := json.Unmarshal(msg.Body, &ev); err != nil || ev.TripID == "" {
				log.Printf("dispatch: malformed trip event, discarding: %s", msg.Body)
				msg.Nack(false, false)
				continue
			}
			handleEvent(ev, pending, active)
			msg.Ack(false)
		}
	}
}
