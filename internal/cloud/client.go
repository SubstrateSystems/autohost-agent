package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) post(ctx context.Context, path string, body any) error {
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

type ErrStatus struct{ Code int }

func (e ErrStatus) Error() string {
	return http.StatusText(e.Code)
}

type HeartbeatPayload struct {
	NodeID        string   `json:"node_id"`
	Hostname      string   `json:"hostname"`
	Tags          []string `json:"tags"`
	OS            string   `json:"os"`
	UptimeSeconds uint64   `json:"uptime_seconds"`
}

func (c *Client) SendHeartbeat(ctx context.Context, hb HeartbeatPayload) error {
	return c.post(ctx, "/v1/heartbeats/heartbeat", hb)
}
