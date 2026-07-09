package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

type LocationPing struct {
	DriverID string  `json:"driverId"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var ping LocationPing
	if err := json.NewDecoder(r.Body).Decode(&ping); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if ping.DriverID == "" {
		http.Error(w, "driverId is required", http.StatusBadRequest)
		return
	}

	log.Printf("location ping: driverId=%s lat=%f lng=%f", ping.DriverID, ping.Lat, ping.Lng)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "received"})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/ping", pingHandler)
	http.HandleFunc("/health", healthHandler)

	log.Printf("location service listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
