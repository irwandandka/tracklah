package main

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestDistanceMeters(t *testing.T) {
	// 0.01 degrees of latitude is ~1112 m everywhere.
	got := distanceMeters(Point{-6.20, 106.8}, Point{-6.21, 106.8})
	if math.Abs(got-1112) > 2 {
		t.Fatalf("distance = %.1f m, want ~1112 m", got)
	}
	if d := distanceMeters(Point{1, 2}, Point{1, 2}); d != 0 {
		t.Fatalf("distance to self = %f, want 0", d)
	}
}

func TestStepTowardMovesExactlyOneStep(t *testing.T) {
	from, to := Point{-6.20, 106.8}, Point{-6.21, 106.81}
	next, arrived := stepToward(from, to, 100)
	if arrived {
		t.Fatal("arrived after one 100 m step on a ~1.5 km leg")
	}
	if d := distanceMeters(from, next); math.Abs(d-100) > 0.5 {
		t.Fatalf("step = %.2f m, want 100 m", d)
	}
	// And it got closer, not just moved.
	if distanceMeters(next, to) >= distanceMeters(from, to) {
		t.Fatal("step did not move toward the target")
	}
}

func TestStepTowardSnapsToTargetWhenClose(t *testing.T) {
	to := Point{-6.2001, 106.8}
	next, arrived := stepToward(Point{-6.2, 106.8}, to, 100)
	if !arrived || next != to {
		t.Fatalf("got %v arrived=%v, want exactly %v and arrived", next, arrived, to)
	}
}

func TestRandomPointNearStaysInRadius(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	center := Point{-6.2, 106.8}
	for i := 0; i < 1000; i++ {
		p := randomPointNear(rng, center, 1500)
		if d := distanceMeters(center, p); d > 1501 {
			t.Fatalf("point %v is %.1f m away, want <= 1500 m", p, d)
		}
	}
}
