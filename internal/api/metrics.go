package api

import (
	"autohost-agent/internal/domain"
	"context"
)

// SendMetrics sends node metrics to the API.
func (c *Client) SendMetrics(ctx context.Context, metrics *domain.Metrics) error {
	return c.post(ctx, EndpointMetrics, metrics)
}
