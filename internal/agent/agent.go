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

	// Enviar métricas inicial
	if err := a.sendMetrics(ctx); err != nil {
		log.Printf("error sending initial metrics: %v", err)
	} else {
		log.Println("Initial metrics sent successfully")
	}

	heartbeatTicker := time.NewTicker(15 * time.Second)
	metricsTicker := time.NewTicker(15 * time.Second)
	defer heartbeatTicker.Stop()
	defer metricsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Agent shutting down...")
			return ctx.Err()
		case <-heartbeatTicker.C:
			if err := a.sendHeartbeat(ctx); err != nil {
				log.Printf("error sending heartbeat: %v", err)
			}
		case <-metricsTicker.C:
			if err := a.sendMetrics(ctx); err != nil {
				log.Printf("error sending metrics: %v", err)
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

func (a *Agent) sendMetrics(ctx context.Context) error {
	metrics, err := system.GetMetrics()
	if err != nil {
		return err
	}

	payload := cloud.MetricsPayload{
		CPUUsagePercent:      metrics.CPUUsagePercent,
		MemoryTotalBytes:     metrics.MemoryTotalBytes,
		MemoryUsedBytes:      metrics.MemoryUsedBytes,
		MemoryAvailableBytes: metrics.MemoryAvailableBytes,
		MemoryUsagePercent:   metrics.MemoryUsagePercent,
		DiskTotalBytes:       metrics.DiskTotalBytes,
		DiskUsedBytes:        metrics.DiskUsedBytes,
		DiskAvailableBytes:   metrics.DiskAvailableBytes,
		DiskUsagePercent:     metrics.DiskUsagePercent,
	}

	return a.cl.SendMetrics(ctx, payload)
}
