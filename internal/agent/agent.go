package agent

import (
	"context"
	"log"

	"autohost-agent/internal/api"
	"autohost-agent/internal/commands"
	"autohost-agent/internal/domain"
	"autohost-agent/internal/services"
	"autohost-agent/internal/transport"
)

// Agent is the top-level orchestrator that coordinates all subsystems:
// gRPC (heartbeats, metrics, command reception) and job execution.
type Agent struct {
	cfg        *Config
	grpcClient *transport.GRPCClient
	registry   *commands.Registry
}

// New creates and wires all agent subsystems.
func New(cfg *Config) *Agent {
	apiClient := api.NewClient(cfg.APIURL, cfg.AgentToken)
	metricsService := services.NewMetricsService()

	registry := commands.NewRegistry()
	commands.RegisterAll(registry)
	if err := commands.RegisterCustomScripts(registry, domain.CustomCommandsDir); err != nil {
		log.Printf("⚠️  could not load custom scripts: %v", err)
	}

	grpcClient := transport.NewGRPCClient(
		cfg.GRPCAddress,
		cfg.AgentToken,
		cfg.NodeID,
		cfg.Tags,
		registry,
		metricsService,
	)

	// Keep the HTTP API client available for operations that still use HTTP
	// (e.g. initial enrollment). Cast to suppress unused-variable warning.
	_ = apiClient

	return &Agent{
		cfg:        cfg,
		grpcClient: grpcClient,
		registry:   registry,
	}
}

// Run starts the agent main loop: gRPC connection with heartbeats and metrics.
func (a *Agent) Run(ctx context.Context) error {
	log.Printf("Agent starting — NodeID: %s, gRPC: %s", a.cfg.NodeID, a.cfg.GRPCAddress)

	if a.cfg.GRPCAddress == "" {
		log.Println("ℹ️  gRPC address not configured — agent is idle")
		<-ctx.Done()
		return ctx.Err()
	}

	if err := a.grpcClient.Run(ctx); err != nil {
		log.Printf("gRPC client stopped: %v", err)
	}

	return ctx.Err()
}
