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
	driversOnline = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "tracklah_sim_drivers_online",
		Help: "Simulated drivers currently on shift.",
	})
	pingsPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tracklah_sim_location_pings_total",
		Help: "Location pings published to RabbitMQ, by result (ok, error).",
	}, []string{"result"})
	tripOutcomes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tracklah_sim_trips_total",
		Help: "Trips a simulated driver picked up, by how they ended: completed, conflict (409, someone else won), cancelled (rider cancelled on the way), error.",
	}, []string{"outcome"})
	dispatchDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tracklah_sim_dispatch_dropped_total",
		Help: "Requested trips dropped because the pending backlog was full.",
	})
)

// watchPending exposes how many requested trips are waiting for a free
// driver. A backlog that keeps growing means every driver is busy or off.
func watchPending(pending chan string) {
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "tracklah_sim_pending_trips",
		Help: "Requested trips waiting for an idle online driver.",
	}, func() float64 { return float64(len(pending)) })
}

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
