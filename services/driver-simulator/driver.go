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
	id         string
	pos        Point
	home       Point // idle roaming stays around here
	api        tripAPI
	ping       pingFunc
	interval   time.Duration
	stepMeters float64
	rng        *rand.Rand
	active     *activeTrips
}

// run is the driver's whole life: roam while idle, and take the next
// requested trip from `pending` whenever it's free. Because only idle
// drivers read from `pending`, the Go channel itself does the dispatching.
func (d *driver) run(ctx context.Context, pending <-chan string) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	waypoint := randomPointNear(d.rng, d.home, idleRoamRadiusMeters)

	for {
		select {
		case <-ctx.Done():
			return
		case tripID := <-pending:
			d.handleTrip(ctx, tripID)
		case <-ticker.C:
			var arrived bool
			d.pos, arrived = stepToward(d.pos, waypoint, d.stepMeters)
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
		log.Printf("[%s] trip %s: stopped before pickup (%v)", d.id, tripID, context.Cause(tripCtx))
		return
	}
	if !d.transition(ctx, tripID, "in_progress", "") {
		return
	}
	log.Printf("[%s] trip %s: picked up rider", d.id, tripID)

	// in_progress can't be cancelled (see the API state machine), so only
	// shutdown stops this leg.
	if err := d.driveTo(ctx, t.destination()); err != nil {
		return
	}
	if d.transition(ctx, tripID, "completed", "") {
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
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var arrived bool
			d.pos, arrived = stepToward(d.pos, target, d.stepMeters)
			d.sendPing()
			if arrived {
				return nil
			}
		}
	}
}

func (d *driver) sendPing() {
	if err := d.ping(d.id, d.pos); err != nil {
		log.Printf("[%s] failed to publish location ping: %v", d.id, err)
	}
}
