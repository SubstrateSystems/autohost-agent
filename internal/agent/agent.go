package agent

import (
	"context"
	"log"
	"os"
	"runtime"
	"time"

	"autohost-agent/internal/cloud"
	"autohost-agent/internal/config"
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
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		if err := a.sendHeartbeat(ctx); err != nil {
			log.Printf("error sending heartbeat: %v", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// loop
		}
	}
}

func (a *Agent) sendHeartbeat(ctx context.Context) error {
	hostname, _ := os.Hostname()
	// luego puedes reemplazar esto con uptime real
	uptime := uint64(time.Now().Unix())

	hb := cloud.HeartbeatPayload{
		NodeID:        a.cfg.NodeID,
		Hostname:      hostname,
		Tags:          a.cfg.Tags,
		OS:            runtime.GOOS,
		UptimeSeconds: uptime,
	}
	return a.cl.SendHeartbeat(ctx, hb)
}
