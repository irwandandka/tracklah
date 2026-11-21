package main

import (
	"context"
	"math/rand/v2"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseDurationRange(t *testing.T) {
	r, err := parseDurationRange("20m-40m")
	if err != nil || r != (durationRange{20 * time.Minute, 40 * time.Minute}) {
		t.Fatalf("got %v, %v", r, err)
	}
	r, err = parseDurationRange("30s")
	if err != nil || r != (durationRange{30 * time.Second, 30 * time.Second}) {
		t.Fatalf("single value: got %v, %v", r, err)
	}
	for _, bad := range []string{"", "abc", "40m-20m", "0s", "-5m"} {
		if _, err := parseDurationRange(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestDurationRangePickStaysInBounds(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	r := durationRange{10 * time.Minute, 30 * time.Minute}
	for i := 0; i < 1000; i++ {
		if d := r.pick(rng); d < r.Min || d > r.Max {
			t.Fatalf("picked %s, outside %s-%s", d, r.Min, r.Max)
		}
	}
}

// A driver must stop pinging while offline (that's what takes it off the
// app's map) and come back online afterwards.
func TestDriverGoesOfflineAndComesBack(t *testing.T) {
	var pings atomic.Int64
	d, _ := newTestDriver(&fakeAPI{})
	d.idle.interval = 5 * time.Millisecond
	d.ping = func(string, Point) error { pings.Add(1); return nil }
	d.shifts = shiftPlan{
		online:  durationRange{60 * time.Millisecond, 60 * time.Millisecond},
		offline: durationRange{200 * time.Millisecond, 200 * time.Millisecond},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.run(ctx, make(chan string))

	time.Sleep(100 * time.Millisecond) // online shift over, now offline
	offlineStart := pings.Load()
	if offlineStart == 0 {
		t.Fatal("no pings during the online shift")
	}
	time.Sleep(100 * time.Millisecond) // still offline
	if got := pings.Load(); got != offlineStart {
		t.Fatalf("pinged %d times while offline", got-offlineStart)
	}
	time.Sleep(200 * time.Millisecond) // back online
	if pings.Load() == offlineStart {
		t.Fatal("driver never came back online")
	}
}

// Offline drivers must not take trips; the trip waits for someone online.
func TestOfflineDriverDoesNotTakeTrips(t *testing.T) {
	api := &fakeAPI{trip: requestedTrip()}
	d, _ := newTestDriver(api)
	d.shifts = shiftPlan{
		online:  durationRange{time.Millisecond, time.Millisecond},
		offline: durationRange{time.Hour, time.Hour},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pending := make(chan string, 1)
	go d.run(ctx, pending)

	time.Sleep(50 * time.Millisecond) // well into the offline shift
	pending <- "trip-1"
	time.Sleep(50 * time.Millisecond)
	if calls := api.statusCalls(); len(calls) != 0 {
		t.Fatalf("offline driver took the trip: %v", calls)
	}
	if len(pending) != 1 {
		t.Fatal("trip should still be waiting in pending")
	}
}
