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
	"autohost-agent/pkg/sysinfo"
)

// Agent is the top-level orchestrator that coordinates all subsystems:
// gRPC (heartbeats, metrics, command reception) and job execution.
type Agent struct {
	cfg             *Config
	configPath      string
	version         string
	apiClient       *api.Client
	grpcClient      *transport.GRPCClient
	registry        *commands.Registry
	lastKnownTokens struct {
		access  string
		refresh string
	}
}

// New creates and wires all agent subsystems.
func New(cfg *Config, version string) *Agent {
	var apiClient *api.Client
	if cfg.RefreshToken != "" {
		apiClient = api.NewClientWithRefresh(cfg.APIURL, cfg.AgentToken, cfg.RefreshToken)
	} else {
		apiClient = api.NewClient(cfg.APIURL, cfg.AgentToken)
	}

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

	a := &Agent{
		cfg:        cfg,
		configPath: "/etc/autohost/config.yaml",
		version:    version,
		apiClient:  apiClient,
		grpcClient: grpcClient,
		registry:   registry,
	}
	a.lastKnownTokens.access = cfg.AgentToken
	a.lastKnownTokens.refresh = cfg.RefreshToken
	return a
}

// Run starts the agent: establishes gRPC connection which handles heartbeats,
// metrics, and command reception internally.
func (a *Agent) Run(ctx context.Context) error {
	log.Printf("Agent starting — NodeID: %s, gRPC: %s", a.cfg.NodeID, a.cfg.GRPCAddress)

	// Periodically return unused memory pages to the OS to keep RSS stable
	// on long-running VPS deployments.
	startMemoryTrimLoop(ctx)

	// Report version to the API on every startup so the frontend stays up to date.
	if a.version != "" {
		if err := a.apiClient.ReportVersion(ctx, a.version); err != nil {
			log.Printf("⚠️  could not report version to API: %v", err)
		} else {
			log.Printf("✅ Version %s reported to API", a.version)
		}
	}

	// Start background network stats reporter (every 30 s).
	go a.networkStatsLoop(ctx)

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

// networkStatsLoop collects network interface stats and listening ports from
// /proc (kernel-native, no external tools) every 30 seconds and POSTs them
// to the API for display in the topology map.
func (a *Agent) networkStatsLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Send once immediately on startup.
	a.sendNetworkStats(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.sendNetworkStats(ctx)
		}
	}
}

func (a *Agent) sendNetworkStats(ctx context.Context) {
	ifaces, err := sysinfo.GetNetworkStats()
	if err != nil {
		log.Printf("⚠️  network stats: %v", err)
		return
	}
	ports, err := sysinfo.GetListeningPorts()
	if err != nil {
		log.Printf("⚠️  listening ports: %v", err)
		return
	}
	peers, _ := sysinfo.GetVPNPeers()
	conns, _ := sysinfo.GetActiveConnections()
	reqs, rps, _ := sysinfo.GetRecentHTTPRequests()

	payload := &api.NetworkStatsPayload{
		Interfaces:  ifaces,
		Ports:       ports,
		Peers:       peers,
		Connections: conns,
		Requests:    reqs,
		RPS:         rps,
	}
	if err := a.apiClient.SendNetworkStats(ctx, payload); err != nil {
		log.Printf("⚠️  send network stats: %v", err)
	}
}

