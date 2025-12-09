package agent

import (
	"context"
	"log"
	"os"
	"runtime"
	"time"

	"autohost-agent/internal/cloud"
	"autohost-agent/internal/config"
	"autohost-agent/internal/system"
)

type Agent struct {
	cfg *config.Config
	cl  *cloud.Client
}

func New(cfg *config.Config) *Agent {
	cl := cloud.NewClient(cfg.APIURL, cfg.AgentToken)
	return &Agent{cfg: cfg, cl: cl}
}

func (a *Agent) Run(ctx context.Context) error {
	log.Printf("Agent starting - NodeID: %s, API: %s", a.cfg.NodeID, a.cfg.APIURL)

	// Enviar heartbeat inicial
	if err := a.sendHeartbeat(ctx); err != nil {
		log.Printf("error sending initial heartbeat: %v", err)
	} else {
		log.Println("Initial heartbeat sent successfully")
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Agent shutting down...")
			return ctx.Err()
		case <-ticker.C:
			if err := a.sendHeartbeat(ctx); err != nil {
				log.Printf("error sending heartbeat: %v", err)
			}
		}
	}
}

func (a *Agent) sendHeartbeat(ctx context.Context) error {
	hostname, _ := os.Hostname()

	uptime, err := system.GetUptimeSeconds()
	if err != nil {
		log.Printf("warning: could not get uptime: %v", err)
		uptime = 0
	}

	hb := cloud.HeartbeatPayload{
		NodeID:        a.cfg.NodeID,
		Hostname:      hostname,
		Tags:          a.cfg.Tags,
		OS:            runtime.GOOS,
		UptimeSeconds: uptime,
	}
	return a.cl.SendHeartbeat(ctx, hb)
}
