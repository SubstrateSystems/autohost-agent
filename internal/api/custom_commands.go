package api

import (
	"context"

	"autohost-agent/internal/domain"
)

// RegisterCustomCommand sends a custom command registration to the API.
func (c *Client) RegisterCustomCommand(ctx context.Context, cmd domain.CustomCommand) error {
	return c.post(ctx, EndpointCustomCommands, cmd)
}
