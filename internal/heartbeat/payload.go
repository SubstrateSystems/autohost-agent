package heartbeat

// Payload represents the heartbeat information sent to the API.
type Payload struct {
	NodeID        string   `json:"node_id"`
	Hostname      string   `json:"hostname"`
	Tags          []string `json:"tags"`
	OS            string   `json:"os"`
	UptimeSeconds uint64   `json:"uptime_seconds"`
}
