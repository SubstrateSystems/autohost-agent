package heartbeat

import (
	"context"
	"log"
	"os"
	"runtime"

	"autohost-agent/pkg/sysinfo"
)

// Config interface defines what heartbeat service needs from configuration.
type Config interface {
	GetNodeID() string
	GetTags() []string
}

// Service handles heartbeat operations.
type Service struct {
	cfg    Config
	sender Sender
}

// Sender is an interface for sending heartbeat payloads.
type Sender interface {
	SendHeartbeat(ctx context.Context, payload Payload) error
}

// NewService creates a new heartbeat service.
func NewService(cfg Config, sender Sender) *Service {
	return &Service{
		cfg:    cfg,
		sender: sender,
	}
}

// Send creates and sends a heartbeat payload.
func (s *Service) Send(ctx context.Context) error {
	hostname, _ := os.Hostname()

	uptime, err := sysinfo.GetUptimeSeconds()
	if err != nil {
		log.Printf("warning: could not get uptime: %v", err)
		uptime = 0
	}

	payload := Payload{
		NodeID:        s.cfg.GetNodeID(),
		Hostname:      hostname,
		Tags:          s.cfg.GetTags(),
		OS:            runtime.GOOS,
		UptimeSeconds: uptime,
	}

	return s.sender.SendHeartbeat(ctx, payload)
}
