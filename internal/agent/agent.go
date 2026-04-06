package agent

import (
	"context"
	"log"
	"time"

	"autohost-agent/internal/api"
	"autohost-agent/internal/commands"
	"autohost-agent/internal/domain"
	"autohost-agent/internal/services"
	"autohost-agent/internal/transport"
)

// Agent is the top-level orchestrator that coordinates all subsystems:
// heartbeats, metrics collection, gRPC command reception, and job execution.
type Agent struct {
	cfg               *Config
	apiClient         *api.Client
	heartbeatService  *services.HeartbeatService
	metricsService    *services.MetricsService
	grpcClient        *transport.GRPCClient
	registry          *commands.Registry
	heartbeatInterval time.Duration
	metricsInterval   time.Duration
}

// New creates and wires all agent subsystems.
func New(cfg *Config) *Agent {
	apiClient := api.NewClient(cfg.APIURL, cfg.AgentToken)
	heartbeatService := services.NewHeartbeatService(cfg, apiClient)
	metricsService := services.NewMetricsService()

	registry := commands.NewRegistry()
	commands.RegisterAll(registry)
	if err := commands.RegisterCustomScripts(registry, domain.CustomCommandsDir); err != nil {
		log.Printf("⚠️  could not load custom scripts: %v", err)
	}

	grpcClient := transport.NewGRPCClient(cfg.GRPCAddress, cfg.AgentToken, cfg.NodeID, registry)

	return &Agent{
		cfg:               cfg,
		apiClient:         apiClient,
		heartbeatService:  heartbeatService,
		metricsService:    metricsService,
		grpcClient:        grpcClient,
		registry:          registry,
		heartbeatInterval: 15 * time.Second,
		metricsInterval:   15 * time.Second,
	}
}

// Run starts the agent main loop: heartbeats, metrics, and WebSocket listener.
func (a *Agent) Run(ctx context.Context) error {
	log.Printf("Agent starting - NodeID: %s, API: %s", a.cfg.NodeID, a.cfg.APIURL)

	// Send initial heartbeat and metrics immediately.
	if err := a.heartbeatService.Send(ctx); err != nil {
		log.Printf("error sending initial heartbeat: %v", err)
	} else {
		log.Println("Initial heartbeat sent successfully")
	}

	if err := a.sendMetrics(ctx); err != nil {
		log.Printf("error sending initial metrics: %v", err)
	} else {
		log.Println("Initial metrics sent successfully")
	}

	// Connect via gRPC for job reception (only if an address is configured).
	if a.cfg.GRPCAddress != "" {
		go func() {
			if err := a.grpcClient.Run(ctx); err != nil {
				log.Printf("gRPC client stopped: %v", err)
			}
		}()
	} else {
		log.Println("ℹ️  gRPC address not configured — command reception disabled")
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

func (a *Agent) sendMetrics(ctx context.Context) error {
	metrics, err := a.metricsService.Collect()
	if err != nil {
		return err
	}
	return a.apiClient.SendMetrics(ctx, metrics)
}
