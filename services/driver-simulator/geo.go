package main

import (
	"math"
	"math/rand/v2"
)

const earthRadiusMeters = 6_371_000.0

type Point struct {
	Lat float64
	Lng float64
}

// distanceMeters is the great-circle (haversine) distance between a and b.
func distanceMeters(a, b Point) float64 {
	lat1, lat2 := a.Lat*math.Pi/180, b.Lat*math.Pi/180
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLng := (b.Lng - a.Lng) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusMeters * math.Asin(math.Sqrt(h))
}

// stepToward moves at most maxMeters from `from` in a straight line to
// `to`, and reports whether it arrived. Linear interpolation in lat/lng is
// fine at city scale (a few km): the error is far below GPS noise.
func stepToward(from, to Point, maxMeters float64) (Point, bool) {
	d := distanceMeters(from, to)
	if d <= maxMeters {
		return to, true
	}
	f := maxMeters / d
	return Point{
		Lat: from.Lat + (to.Lat-from.Lat)*f,
		Lng: from.Lng + (to.Lng-from.Lng)*f,
	}, false
}

// randomPointNear picks a uniformly distributed point within radiusMeters
// of center (sqrt on the radius so points don't bunch up in the middle).
func randomPointNear(rng *rand.Rand, center Point, radiusMeters float64) Point {
	r := radiusMeters * math.Sqrt(rng.Float64())
	theta := rng.Float64() * 2 * math.Pi
	dLat := (r * math.Cos(theta)) / earthRadiusMeters * 180 / math.Pi
	dLng := (r * math.Sin(theta)) / (earthRadiusMeters * math.Cos(center.Lat*math.Pi/180)) * 180 / math.Pi
	return Point{Lat: center.Lat + dLat, Lng: center.Lng + dLng}
}
