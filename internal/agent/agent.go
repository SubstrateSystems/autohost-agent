package agent

import (
	"context"
	"log"
	"time"

	"autohost-agent/internal/heartbeat"
	"autohost-agent/internal/metrics"
	"autohost-agent/internal/transport"
)

// Agent represents the main agent orchestrator.
type Agent struct {
	cfg               *Config
	heartbeatService  *heartbeat.Service
	metricsCollector  *metrics.Collector
	httpClient        *transport.HTTPClient
	heartbeatInterval time.Duration
	metricsInterval   time.Duration
}

// New creates a new agent instance.
func New(cfg *Config) *Agent {
	httpClient := transport.NewHTTPClient(cfg.APIURL, cfg.AgentToken)
	heartbeatService := heartbeat.NewService(cfg, httpClient)
	metricsCollector := metrics.NewCollector()

	return &Agent{
		cfg:               cfg,
		heartbeatService:  heartbeatService,
		metricsCollector:  metricsCollector,
		httpClient:        httpClient,
		heartbeatInterval: 15 * time.Second,
		metricsInterval:   15 * time.Second,
	}
}

// Run starts the agent lifecycle.
func (a *Agent) Run(ctx context.Context) error {
	log.Printf("Agent starting - NodeID: %s, API: %s", a.cfg.NodeID, a.cfg.APIURL)

	// Enviar heartbeat inicial
	if err := a.heartbeatService.Send(ctx); err != nil {
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

	heartbeatTicker := time.NewTicker(a.heartbeatInterval)
	metricsTicker := time.NewTicker(a.metricsInterval)
	defer heartbeatTicker.Stop()
	defer metricsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Agent shutting down...")
			return ctx.Err()
		case <-heartbeatTicker.C:
			if err := a.heartbeatService.Send(ctx); err != nil {
				log.Printf("error sending heartbeat: %v", err)
			}
		case <-metricsTicker.C:
			if err := a.sendMetrics(ctx); err != nil {
				log.Printf("error sending metrics: %v", err)
			}
		}
	}
}

// sendMetrics collects and sends metrics.
func (a *Agent) sendMetrics(ctx context.Context) error {
	metrics, err := a.metricsCollector.Collect()
	if err != nil {
		return err
	}

	return a.httpClient.SendMetrics(ctx, metrics)
}
