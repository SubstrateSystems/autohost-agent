package api

import (
	"autohost-agent/pkg/sysinfo"
	"context"
)

// NetworkStatsPayload is the JSON body sent to the API network-stats endpoint.
type NetworkStatsPayload struct {
	Interfaces []sysinfo.NetworkInterfaceStats `json:"interfaces"`
	Ports      []sysinfo.ListeningPort         `json:"ports"`
}

// SendNetworkStats posts current network stats and listening ports to the API.
func (c *Client) SendNetworkStats(ctx context.Context, stats *NetworkStatsPayload) error {
	return c.post(ctx, EndpointNetworkStats, stats)
}
