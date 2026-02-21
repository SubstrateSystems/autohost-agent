package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Client handles HTTP communication with the Autohost API.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient creates a new API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// post performs an authenticated POST request to the given path.
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
		return &ErrStatus{Code: resp.StatusCode}
	}

	return nil
}
