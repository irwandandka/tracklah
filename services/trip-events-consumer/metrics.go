package main

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// tripEvents counts lifecycle events per downstream consumer. "malformed"
// is used as the status for events that could not be decoded.
var tripEvents = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "tracklah_trip_events_consumed_total",
	Help: "trip.lifecycle events handled, by consumer and trip status.",
}, []string{"consumer", "status"})

// serveMetrics exposes /metrics on its own port for Prometheus.
func serveMetrics(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("metrics server: %v", err)
		}
	}()
}
