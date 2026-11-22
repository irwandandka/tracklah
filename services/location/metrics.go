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

var (
	pingsConsumed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tracklah_location_pings_consumed_total",
		Help: "Location pings taken off RabbitMQ, by result (ok, malformed).",
	}, []string{"result"})
	sinkErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tracklah_location_sink_errors_total",
		Help: "Failed fan-out writes of a ping, by sink (redis, firestore).",
	}, []string{"sink"})
	firestoreWrite = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "tracklah_location_firestore_write_seconds",
		Help:    "Latency of the driverLocations write to Firestore.",
		Buckets: []float64{0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	})
)

// serveMetrics exposes /metrics on its own port for Prometheus, apart
// from the service port that also takes /ping.
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
