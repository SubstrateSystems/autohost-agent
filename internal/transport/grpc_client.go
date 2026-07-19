package transport

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"

	"autohost-agent/internal/commands"
	pb "autohost-agent/internal/grpc/nodepb"
	"autohost-agent/internal/services"
)

type GRPCClient struct {
	address        string
	token          string
	nodeID         string
	tags           []string
	registry       *commands.Registry
	metricsService *services.MetricsService

	heartbeatInterval time.Duration
	metricsInterval   time.Duration

	logMu     sync.Mutex
	logCancel context.CancelFunc // non-nil while log streaming is active
	logDone   <-chan struct{}    // closed when the active log goroutine exits

	containerMu     sync.Mutex
	containerCancel context.CancelFunc // non-nil while container streaming is active

	healthMon *healthMonitor
}

func NewGRPCClient(address, token, nodeID string, tags []string, registry *commands.Registry, metricsService *services.MetricsService) *GRPCClient {
	return &GRPCClient{
		address:           address,
		token:             token,
		nodeID:            nodeID,
		tags:              tags,
		registry:          registry,
		metricsService:    metricsService,
		heartbeatInterval: 30 * time.Second,
		metricsInterval:   5 * time.Second,
		healthMon:         newHealthMonitor(),
	}
}

// Run connects to the gRPC server and maintains the session, reconnecting on
// failure until the context is cancelled.
func (c *GRPCClient) Run(ctx context.Context) error {
	if c.address == "" {
		return fmt.Errorf("gRPC address is empty — skipping")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.runOnce(ctx); err != nil {
			log.Printf("⚠️  gRPC session ended: %v — retrying in 10s", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}

func (c *GRPCClient) runOnce(ctx context.Context) error {
	// gRPC addresses must be "host:port" — strip any http:// or https:// prefix.
	// Use TLS when the address has an https:// scheme (e.g. via Cloudflare Tunnel).
	addr := c.address
	useTLS := strings.HasPrefix(addr, "https://") || strings.HasSuffix(addr, ":443")
	// 2. Limpiar la dirección para gRPC
	addr = strings.TrimPrefix(addr, "https://")
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimRight(addr, "/")

	if !strings.Contains(addr, ":") {
		if useTLS || strings.Contains(addr, ".") {
			addr = addr + ":443"
			useTLS = true
		} else {
			addr = addr + ":9090"
		}
	}

	var transportCreds grpc.DialOption
	if useTLS {
		// Usamos la configuración de TLS por defecto del sistema
		// Esto es necesario para que Cloudflare acepte la conexión
		transportCreds = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			ServerName: strings.Split(addr, ":")[0], // Asegura el SNI correcto
		}))
	} else {
		transportCreds = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	conn, err := grpc.NewClient(addr,
		transportCreds,
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                20 * time.Second, // send PING every 20s when idle
			Timeout:             5 * time.Second,  // wait 5s for PONG before closing
			PermitWithoutStream: true,             // keep alive even with no active RPCs
		}),
	)
	if err != nil {
		return fmt.Errorf("dial %s: %w", c.address, err)
	}
	defer conn.Close()

	client := pb.NewNodeAgentServiceClient(conn)
	authCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.token)

	if err := c.registerCommands(authCtx, client); err != nil {
		return fmt.Errorf("RegisterCommands: %w", err)
	}

	return c.connectStream(authCtx, client)
}

// registerCommands streams all registry commands to the server so it knows
// what this node can do.
func (c *GRPCClient) registerCommands(ctx context.Context, client pb.NodeAgentServiceClient) error {
	stream, err := client.RegisterCommands(ctx)
	if err != nil {
		return err
	}

	for _, info := range c.registry.ListCommands() {
		protoType := pb.CommandType_COMMAND_TYPE_DEFAULT
		if info.Kind == commands.KindCustom {
			protoType = pb.CommandType_COMMAND_TYPE_CUSTOM
		}
		req := &pb.RegisterCommandRequest{
			NodeId:      c.nodeID,
			Name:        info.Name,
			Description: info.Name,
			Type:        protoType,
		}
		if err := stream.Send(req); err != nil {
			return fmt.Errorf("send command %q: %w", info.Name, err)
		}
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("CloseAndRecv: %w", err)
	}
	log.Printf("✅ gRPC: registered %d commands with server", resp.GetRegistered())
	return nil
}

// connectStream opens the bidirectional Connect stream and processes incoming
// ExecuteJob messages, sending back job results.
func (c *GRPCClient) connectStream(ctx context.Context, client pb.NodeAgentServiceClient) error {
	stream, err := client.Connect(ctx)
	if err != nil {
		return fmt.Errorf("Connect: %w", err)
	}
	log.Printf("🔗 gRPC: Connect stream established (node %s)", c.nodeID)

	// streamCtx is cancelled when this function returns (either on EOF or error),
	// ensuring all goroutines tied to this session are stopped on exit.
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer func() {
		streamCancel()
		c.healthMon.stopAll()
		c.stopContainerStream()
	}()

	// Buffered channel so goroutines don't block on slow sends.
	results := make(chan *pb.NodeMessage, 32)

	// Sender: forwards all NodeMessages to the server.
	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		for {
			select {
			case msg, ok := <-results:
				if !ok {
					return
				}
				if err := stream.Send(msg); err != nil {
					log.Printf("gRPC send error: %v", err)
					streamCancel() // signal all goroutines to stop
					return
				}
			case <-streamCtx.Done():
				return
			}
		}
	}()

	// Heartbeat goroutine: sends a HeartbeatPayload every heartbeatInterval.
	go c.heartbeatLoop(streamCtx, results)

	// Metrics goroutine: collects and sends MetricPayload every metricsInterval.
	go c.metricsLoop(streamCtx, results)

	// Receiver: handles ServerMessage frames (execute_job, stream_logs, etc.).
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("Recv: %w", err)
		}

		switch p := msg.GetPayload().(type) {
		case *pb.ServerMessage_ExecuteJob:
			go c.handleJob(streamCtx, p.ExecuteJob, results)
		case *pb.ServerMessage_StreamLogs:
			c.startLogStream(streamCtx, p.StreamLogs, results)
		case *pb.ServerMessage_StopLogs:
			c.stopLogStream()
		case *pb.ServerMessage_StreamContainers:
			c.startContainerStream(streamCtx, results)
		case *pb.ServerMessage_StopContainers:
			c.stopContainerStream()
		case *pb.ServerMessage_ConfigureHealthCheck:
			cfg := healthCheckConfig{
				MonitorID:          p.ConfigureHealthCheck.GetMonitorId(),
				ServiceName:        p.ConfigureHealthCheck.GetServiceName(),
				CheckType:          p.ConfigureHealthCheck.GetCheckType(),
				HTTPURL:            p.ConfigureHealthCheck.GetHttpUrl(),
				HTTPExpectedStatus: int(p.ConfigureHealthCheck.GetHttpExpectedStatus()),
				TCPHost:            p.ConfigureHealthCheck.GetTcpHost(),
				TCPPort:            int(p.ConfigureHealthCheck.GetTcpPort()),
				IntervalSeconds:    int(p.ConfigureHealthCheck.GetIntervalSeconds()),
				FailureThreshold:   int(p.ConfigureHealthCheck.GetFailureThreshold()),
				Enabled:            p.ConfigureHealthCheck.GetEnabled(),
			}
			c.healthMon.upsert(streamCtx, cfg, results)
		case *pb.ServerMessage_StopHealthCheck:
			c.healthMon.remove(p.StopHealthCheck.GetMonitorId())
		default:
			log.Printf("gRPC: unknown ServerMessage payload type %T", p)
		}
	}

	// Cancel the stream context BEFORE closing the results channel so that
	// heartbeatLoop and metricsLoop stop trying to send. Sending to a closed
	// channel panics even inside a select-with-default, so we must guarantee
	// those goroutines have received the cancellation signal first.
	// streamCancel is idempotent; the deferred call below is a no-op after this.
	streamCancel()
	<-sendDone
	return nil
}

// heartbeatLoop sends a HeartbeatPayload to the server at a fixed interval.
func (c *GRPCClient) heartbeatLoop(ctx context.Context, out chan<- *pb.NodeMessage) {
	ticker := time.NewTicker(c.heartbeatInterval)
	defer ticker.Stop()

	// Send the first heartbeat immediately.
	c.sendHeartbeat(out)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sendHeartbeat(out)
		}
	}
}

func (c *GRPCClient) sendHeartbeat(out chan<- *pb.NodeMessage) {
	uptime, err := uptimeSeconds()
	if err != nil {
		uptime = 0
	}
	msg := &pb.NodeMessage{
		Payload: &pb.NodeMessage_Heartbeat{
			Heartbeat: &pb.HeartbeatPayload{
				NodeId: c.nodeID,
			},
		},
	}
	_ = uptime // HeartbeatPayload only carries node_id; uptime goes in MetricPayload
	select {
	case out <- msg:
	default:
		log.Printf("⚠️  gRPC: heartbeat buffer full, skipping")
	}
}

// metricsLoop collects system metrics and sends them to the server periodically.
func (c *GRPCClient) metricsLoop(ctx context.Context, out chan<- *pb.NodeMessage) {
	ticker := time.NewTicker(c.metricsInterval)
	defer ticker.Stop()

	// Send the first batch immediately.
	c.sendMetrics(out)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sendMetrics(out)
		}
	}
}

func (c *GRPCClient) sendMetrics(out chan<- *pb.NodeMessage) {
	m, err := c.metricsService.Collect()
	if err != nil {
		log.Printf("⚠️  gRPC: collect metrics: %v", err)
		return
	}
	uptime, _ := uptimeSeconds()
	msg := &pb.NodeMessage{
		Payload: &pb.NodeMessage_Metric{
			Metric: &pb.MetricPayload{
				CpuUsagePercent:    float32(m.CPUUsagePercent),
				RamTotalBytes:      m.MemoryTotalBytes,
				RamUsedBytes:       m.MemoryUsedBytes,
				RamAvailableBytes:  m.MemoryAvailableBytes,
				RamUsagePercent:    float32(m.MemoryUsagePercent),
				DiskTotalBytes:     m.DiskTotalBytes,
				DiskUsedBytes:      m.DiskUsedBytes,
				DiskAvailableBytes: m.DiskAvailableBytes,
				DiskUsagePercent:   float32(m.DiskUsagePercent),
				UptimeSeconds:      uptime,
			},
		},
	}
	select {
	case out <- msg:
	default:
		log.Printf("⚠️  gRPC: metrics buffer full, skipping")
	}
}

// uptimeSeconds returns the system uptime in seconds.
func uptimeSeconds() (int64, error) {
	// Read /proc/uptime: first field is uptime in seconds (float).
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	var up float64
	_, err = fmt.Sscanf(string(data), "%f", &up)
	if err != nil {
		return 0, err
	}
	return int64(up), nil
}

// handleJob executes the given job and sends the result back through results.
func (c *GRPCClient) handleJob(ctx context.Context, job *pb.ExecuteJobPayload, results chan<- *pb.NodeMessage) {
	log.Printf("⚙️  gRPC job %s: executing command %q", job.GetJobId(), job.GetCommandName())

	// Build optional payload from the proto params field.
	// params is a JSON object string — unmarshal it so commands can access
	// individual keys directly (e.g. payload["domain"]).
	// Also keep the raw JSON string under "params" for commands that read it
	// as a single blob (e.g. MarketplaceInstall).
	var payload map[string]any
	if p := job.GetParams(); p != "" {
		if err := json.Unmarshal([]byte(p), &payload); err != nil {
			payload = map[string]any{}
			log.Printf("⚠️  gRPC job %s: could not parse params JSON: %v", job.GetJobId(), err)
		}
		payload["params"] = p // always expose raw JSON string
	}

	res := c.registry.Execute(ctx, job.GetCommandName(), payload)

	var result *pb.JobResultPayload
	if res.Err != nil {
		log.Printf("❌ gRPC job %s failed: %v", job.GetJobId(), res.Err)
		result = &pb.JobResultPayload{
			JobId:  job.GetJobId(),
			Status: pb.JobStatus_JOB_STATUS_FAILED,
			Output: res.Output,
			Error:  res.Err.Error(),
		}
	} else {
		log.Printf("✅ gRPC job %s completed", job.GetJobId())
		result = &pb.JobResultPayload{
			JobId:  job.GetJobId(),
			Status: pb.JobStatus_JOB_STATUS_COMPLETED,
			Output: res.Output,
		}
	}

	select {
	case results <- &pb.NodeMessage{
		Payload: &pb.NodeMessage_JobResult{JobResult: result},
	}:
	default:
		log.Printf("⚠️  gRPC: result buffer full, dropping result for job %s", job.GetJobId())
	}
}
