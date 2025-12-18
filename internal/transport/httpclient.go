package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"autohost-agent/internal/heartbeat"
	"autohost-agent/internal/metrics"
)

// API Endpoints
const (
	EndpointHeartbeat = "/v1/heartbeats/heartbeat"
	EndpointMetrics   = "/v1/node-metrics/metrics"
)

// HTTPClient handles HTTP communication with the API.
type HTTPClient struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewHTTPClient creates a new HTTP client.
func NewHTTPClient(baseURL, token string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		token:   token,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// post performs a POST request to the given path with the given body.
func (c *HTTPClient) post(ctx context.Context, path string, body any) error {
	bs, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(bs))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return ErrStatus{Code: resp.StatusCode}
	}

	return nil
}

// ErrStatus represents an HTTP error status.
type ErrStatus struct {
	Code int
}

func (e ErrStatus) Error() string {
	return http.StatusText(e.Code)
}

// SendHeartbeat sends a heartbeat payload to the API.
func (c *HTTPClient) SendHeartbeat(ctx context.Context, payload heartbeat.Payload) error {
	return c.post(ctx, EndpointHeartbeat, payload)
}

// SendMetrics sends metrics to the API.
func (c *HTTPClient) SendMetrics(ctx context.Context, metrics *metrics.Metrics) error {
	return c.post(ctx, EndpointMetrics, metrics)
}
