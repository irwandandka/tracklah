package main

import (
	"context"
	"errors"
	"log"
	"math/rand/v2"
	"time"
)

const idleRoamRadiusMeters = 1500.0

// pingFunc publishes one location ping for a driver.
type pingFunc func(driverID string, p Point) error

type driver struct {
	id     string
	pos    Point
	home   Point // idle roaming stays around here
	api    tripAPI
	ping   pingFunc
	idle   motion // waiting for an order
	onTrip motion // heading to pickup or to the destination
	rng    *rand.Rand
	active *activeTrips
	shifts shiftPlan
}

// motion is how often a driver pings and how far it moves between pings.
// Waiting for an order pings less often than being on a trip: dispatch
// only needs a rough position, while a rider watching their driver
// approach wants a smoother marker.
type motion struct {
	interval   time.Duration
	stepMeters float64
}

// durationRange is an inclusive [Min, Max] a random duration is drawn from.
type durationRange struct {
	Min, Max time.Duration
}

func (r durationRange) pick(rng *rand.Rand) time.Duration {
	if r.Max <= r.Min {
		return r.Min
	}
	return r.Min + time.Duration(rng.Int64N(int64(r.Max-r.Min)+1))
}

// shiftPlan makes a driver come and go like a real one: online for a
// while (roaming, pinging, taking trips), then offline (no pings at all,
// so the app's "updatedAt > now-30s" query drops them from the map).
// The zero value means always online.
type shiftPlan struct {
	online  durationRange
	offline durationRange
}

func (s shiftPlan) alwaysOnline() bool { return s.online.Max <= 0 }

// run is the driver's whole life: alternate online and offline shifts.
func (d *driver) run(ctx context.Context, pending <-chan string) {
	for {
		var shift time.Duration
		if !d.shifts.alwaysOnline() {
			shift = d.shifts.online.pick(d.rng)
			log.Printf("[%s] online for %s", d.id, shift.Round(time.Second))
		}
		if !d.online(ctx, pending, shift) {
			return
		}

		off := d.shifts.offline.pick(d.rng)
		log.Printf("[%s] offline for %s", d.id, off.Round(time.Second))
		select {
		case <-ctx.Done():
			return
		case <-time.After(off):
		}
	}
}

// online roams while idle and takes the next requested trip from
// `pending` whenever it's free. Because only idle, online drivers read
// from `pending`, the Go channel itself does the dispatching. It returns
// true when the shift is over (never in the middle of a trip: a driver
// whose shift ends during a trip finishes it first), false on shutdown.
// shift <= 0 means the shift never ends.
func (d *driver) online(ctx context.Context, pending <-chan string, shift time.Duration) bool {
	ticker := time.NewTicker(d.idle.interval)
	defer ticker.Stop()
	var shiftOver <-chan time.Time
	if shift > 0 {
		t := time.NewTimer(shift)
		defer t.Stop()
		shiftOver = t.C
	}
	driversOnline.Inc()
	defer driversOnline.Dec()
	waypoint := randomPointNear(d.rng, d.home, idleRoamRadiusMeters)
	d.sendPing() // show up on the map right away, not one interval later

	for {
		select {
		case <-ctx.Done():
			return false
		case <-shiftOver:
			return true
		case tripID := <-pending:
			d.handleTrip(ctx, tripID)
			ticker.Reset(d.idle.interval)
			// If the shift ended during the trip, log off now instead of
			// letting select pick another pending trip at random.
			select {
			case <-shiftOver:
				return true
			default:
			}
		case <-ticker.C:
			var arrived bool
			d.pos, arrived = stepToward(d.pos, waypoint, d.idle.stepMeters)
			if arrived {
				waypoint = randomPointNear(d.rng, d.home, idleRoamRadiusMeters)
			}
			d.sendPing()
		}
	}
}

// handleTrip runs one trip end to end: assign -> drive to origin ->
// in_progress -> drive to destination -> completed. Any 409 along the way
// means someone else (another driver, or the rider cancelling) won, so the
// driver just drops the trip and goes back to roaming.
func (d *driver) handleTrip(ctx context.Context, tripID string) {
	t, err := d.api.getTrip(ctx, tripID)
	if err != nil {
		log.Printf("[%s] trip %s: lookup failed: %v", d.id, tripID, err)
		return
	}
	if t.Status != "requested" {
		return // cancelled or taken while it sat in the queue
	}

	if !d.transition(ctx, tripID, "assigned", d.id) {
		return
	}
	// From here on the trip is this driver's, so count how it ends.
	outcome := "error"
	defer func() { tripOutcomes.WithLabelValues(outcome).Inc() }()
	log.Printf("[%s] trip %s: assigned, heading to pickup", d.id, tripID)

	// The rider may cancel while the driver is on the way: the dispatcher
	// cancels tripCtx when that event arrives.
	tripCtx, cancel := context.WithCancelCause(ctx)
	d.active.add(tripID, cancel)
	defer func() {
		d.active.remove(tripID)
		cancel(nil)
	}()

	if err := d.driveTo(tripCtx, t.origin()); err != nil {
		if ctx.Err() == nil {
			outcome = "cancelled" // the rider cancelled, not a shutdown
		}
		log.Printf("[%s] trip %s: stopped before pickup (%v)", d.id, tripID, context.Cause(tripCtx))
		return
	}
	if !d.transition(ctx, tripID, "in_progress", "") {
		outcome = "conflict"
		return
	}
	log.Printf("[%s] trip %s: picked up rider", d.id, tripID)

	// in_progress can't be cancelled (see the API state machine), so only
	// shutdown stops this leg.
	if err := d.driveTo(ctx, t.destination()); err != nil {
		return
	}
	if d.transition(ctx, tripID, "completed", "") {
		outcome = "completed"
		log.Printf("[%s] trip %s: completed", d.id, tripID)
	}
}

func (d *driver) transition(ctx context.Context, tripID, status, driverID string) bool {
	err := d.api.setStatus(ctx, tripID, status, driverID)
	switch {
	case err == nil:
		return true
	case errors.Is(err, errConflict):
		log.Printf("[%s] trip %s: %s refused (409), dropping trip", d.id, tripID, status)
	default:
		log.Printf("[%s] trip %s: setting %s failed: %v", d.id, tripID, status, err)
	}
	return false
}

// driveTo moves toward target one step per tick, pinging each time, until
// it arrives or ctx is cancelled.
func (d *driver) driveTo(ctx context.Context, target Point) error {
	ticker := time.NewTicker(d.onTrip.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var arrived bool
			d.pos, arrived = stepToward(d.pos, target, d.onTrip.stepMeters)
			d.sendPing()
			if arrived {
				return nil
			}
		}
	}
}

func (d *driver) sendPing() {
	if err := d.ping(d.id, d.pos); err != nil {
		pingsPublished.WithLabelValues("error").Inc()
		log.Printf("[%s] failed to publish location ping: %v", d.id, err)
		return
	}
	pingsPublished.WithLabelValues("ok").Inc()
}
