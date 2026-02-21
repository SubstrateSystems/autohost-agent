package api

import (
	"autohost-agent/internal/domain"
	"context"
)

// SendHeartbeat sends a heartbeat payload to the API.
func (c *Client) SendHeartbeat(ctx context.Context, payload domain.HeartbeatPayload) error {
	return c.post(ctx, EndpointHeartbeat, payload)
}
