package main

import (
	"context"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
	"time"
)

// fakeAPI records status changes and can refuse some with errConflict.
type fakeAPI struct {
	mu       sync.Mutex
	trip     trip
	calls    []string
	refuse   map[string]bool
	onStatus func(status string)
}

func (f *fakeAPI) getTrip(_ context.Context, id string) (trip, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.trip, nil
}

func (f *fakeAPI) setStatus(_ context.Context, id, status, driverID string) error {
	f.mu.Lock()
	f.calls = append(f.calls, status+":"+driverID)
	refused := f.refuse[status]
	if !refused {
		f.trip.Status = status
	}
	hook := f.onStatus
	f.mu.Unlock()
	if refused {
		return errConflict
	}
	if hook != nil {
		hook(status)
	}
	return nil
}

func (f *fakeAPI) statusCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func newTestDriver(api tripAPI) (*driver, *[]Point) {
	var mu sync.Mutex
	pings := &[]Point{}
	start := Point{-6.20, 106.80}
	return &driver{
		id: "driver-1", pos: start, home: start,
		api: api,
		ping: func(_ string, p Point) error {
			mu.Lock()
			defer mu.Unlock()
			*pings = append(*pings, p)
			return nil
		},
		interval:   time.Millisecond,
		stepMeters: 500,
		rng:        rand.New(rand.NewPCG(1, 2)),
		active:     newActiveTrips(),
	}, pings
}

func requestedTrip() trip {
	return trip{
		ID: "trip-1", Status: "requested",
		OriginLat: -6.205, OriginLng: 106.805,
		DestinationLat: -6.215, DestinationLng: 106.815,
	}
}

func TestHandleTripRunsFullLifecycle(t *testing.T) {
	api := &fakeAPI{trip: requestedTrip()}
	d, pings := newTestDriver(api)

	d.handleTrip(context.Background(), "trip-1")

	want := []string{"assigned:driver-1", "in_progress:", "completed:"}
	if got := api.statusCalls(); !slices.Equal(got, want) {
		t.Fatalf("status calls = %v, want %v", got, want)
	}
	if d.pos != requestedTrip().destination() {
		t.Fatalf("driver ended at %v, want destination", d.pos)
	}
	if len(*pings) == 0 {
		t.Fatal("driver never pinged while driving")
	}
}

func TestHandleTripSkipsTripThatIsNoLongerRequested(t *testing.T) {
	tr := requestedTrip()
	tr.Status = "cancelled"
	api := &fakeAPI{trip: tr}
	d, _ := newTestDriver(api)

	d.handleTrip(context.Background(), "trip-1")

	if calls := api.statusCalls(); len(calls) != 0 {
		t.Fatalf("expected no status calls, got %v", calls)
	}
}

func TestHandleTripDropsTripWhenAssignIsRefused(t *testing.T) {
	api := &fakeAPI{trip: requestedTrip(), refuse: map[string]bool{"assigned": true}}
	d, _ := newTestDriver(api)
	start := d.pos

	d.handleTrip(context.Background(), "trip-1")

	if got := api.statusCalls(); !slices.Equal(got, []string{"assigned:driver-1"}) {
		t.Fatalf("status calls = %v, want only the refused assign", got)
	}
	if d.pos != start {
		t.Fatal("driver moved for a trip it never got")
	}
}

func TestHandleTripStopsWhenRiderCancelsBeforePickup(t *testing.T) {
	api := &fakeAPI{trip: requestedTrip()}
	d, _ := newTestDriver(api)
	d.stepMeters = 1 // slow enough that the cancel lands mid-drive
	api.onStatus = func(status string) {
		if status == "assigned" {
			go func() {
				// Wait until handleTrip has registered the trip.
				for !d.active.cancel("trip-1") {
					time.Sleep(time.Millisecond)
				}
			}()
		}
	}

	done := make(chan struct{})
	go func() { d.handleTrip(context.Background(), "trip-1"); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleTrip did not stop after the rider cancelled")
	}

	if got := api.statusCalls(); !slices.Equal(got, []string{"assigned:driver-1"}) {
		t.Fatalf("status calls = %v, want only assigned", got)
	}
	if d.active.cancel("trip-1") {
		t.Fatal("trip still registered as active after it ended")
	}
}

func TestHandleEvent(t *testing.T) {
	pending := make(chan string, 1)
	active := newActiveTrips()

	handleEvent(tripEvent{TripID: "a", Status: "requested"}, pending, active)
	if got := <-pending; got != "a" {
		t.Fatalf("pending got %q, want a", got)
	}

	// Our own updates echo back through the fanout and must be ignored.
	handleEvent(tripEvent{TripID: "a", Status: "assigned"}, pending, active)
	if len(pending) != 0 {
		t.Fatal("non-requested event was queued")
	}

	// A full backlog drops instead of blocking the consumer.
	pending <- "x"
	handleEvent(tripEvent{TripID: "b", Status: "requested"}, pending, active)

	cancelled := false
	ctx, cancel := context.WithCancelCause(context.Background())
	active.add("c", func(err error) { cancelled = true; cancel(err) })
	handleEvent(tripEvent{TripID: "c", Status: "cancelled"}, pending, active)
	if !cancelled || context.Cause(ctx) != errCancelledByRider {
		t.Fatal("cancelled event did not stop the active driver")
	}
}
