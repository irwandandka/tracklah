package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// errConflict means the API refused the change with 409: the trip was
// taken by another driver, cancelled by the rider, or the transition is
// no longer valid. It's an expected outcome, not a failure.
var errConflict = errors.New("trip state conflict")

type trip struct {
	ID             string  `json:"id"`
	Status         string  `json:"status"`
	DriverID       string  `json:"driverId"`
	OriginLat      float64 `json:"originLat"`
	OriginLng      float64 `json:"originLng"`
	DestinationLat float64 `json:"destinationLat"`
	DestinationLng float64 `json:"destinationLng"`
}

func (t trip) origin() Point      { return Point{t.OriginLat, t.OriginLng} }
func (t trip) destination() Point { return Point{t.DestinationLat, t.DestinationLng} }

// tripAPI is the slice of the NestJS API the simulator needs. An interface
// so the driver logic can be tested without HTTP.
type tripAPI interface {
	getTrip(ctx context.Context, id string) (trip, error)
	setStatus(ctx context.Context, id, status, driverID string) error
}

type httpTripAPI struct {
	baseURL string
	client  *http.Client
	retries int
	backoff time.Duration
}

func newHTTPTripAPI(baseURL string) *httpTripAPI {
	return &httpTripAPI{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 5 * time.Second},
		retries: 3,
		backoff: time.Second,
	}
}

func (a *httpTripAPI) getTrip(ctx context.Context, id string) (trip, error) {
	var t trip
	err := a.do(ctx, http.MethodGet, "/trips/"+id, nil, &t)
	return t, err
}

func (a *httpTripAPI) setStatus(ctx context.Context, id, status, driverID string) error {
	body := map[string]string{"status": status}
	if driverID != "" {
		body["driverId"] = driverID
	}
	return a.do(ctx, http.MethodPatch, "/trips/"+id+"/status", body, nil)
}

// do retries network errors and 5xx with exponential backoff. 4xx are
// returned straight away since retrying won't change the answer.
func (a *httpTripAPI) do(ctx context.Context, method, path string, in, out any) error {
	var lastErr error
	for attempt := 0; attempt <= a.retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(a.backoff << (attempt - 1)):
			}
		}
		retry, err := a.once(ctx, method, path, in, out)
		if err == nil || !retry {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("%s %s: giving up after %d attempts: %w", method, path, a.retries+1, lastErr)
}

func (a *httpTripAPI) once(ctx context.Context, method, path string, in, out any) (retry bool, err error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return false, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, body)
	if err != nil {
		return false, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return ctx.Err() == nil, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusConflict:
		return false, errConflict
	case resp.StatusCode >= 500:
		return true, fmt.Errorf("%s %s: %s", method, path, resp.Status)
	case resp.StatusCode >= 400:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return false, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, msg)
	}
	if out != nil {
		return false, json.NewDecoder(resp.Body).Decode(out)
	}
	return false, nil
}
