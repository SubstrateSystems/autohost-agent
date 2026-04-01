package transport

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"autohost-agent/internal/commands"
	pb "autohost-agent/internal/grpc/nodepb"
)

// GRPCClient manages the long-lived gRPC connection to cloud-api.
// It handles RegisterCommands on startup and the bidirectional Connect stream
// for job dispatch and result reporting.
type GRPCClient struct {
	address  string
	token    string
	nodeID   string
	registry *commands.Registry

	logMu     sync.Mutex
	logCancel context.CancelFunc // non-nil while log streaming is active
}

// NewGRPCClient creates a new GRPCClient.
func NewGRPCClient(address, token, nodeID string, registry *commands.Registry) *GRPCClient {
	return &GRPCClient{
		address:  address,
		token:    token,
		nodeID:   nodeID,
		registry: registry,
	}
}

// Run connects to the gRPC server and maintains the session, reconnecting on
// failure until the context is cancelled.
func (c *GRPCClient) Run(ctx context.Context) error {
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
	addr := c.address
	addr = strings.TrimPrefix(addr, "https://")
	addr = strings.TrimPrefix(addr, "http://")

	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
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

	// Buffered channel so job goroutines don't block on slow sends.
	results := make(chan *pb.NodeMessage, 16)

	// Sender: forwards job results to the server.
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
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Receiver: handles ServerMessage frames (execute_job).
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
			go c.handleJob(ctx, p.ExecuteJob, results)
		case *pb.ServerMessage_StreamLogs:
			c.startLogStream(ctx, p.StreamLogs, results)
		case *pb.ServerMessage_StopLogs:
			c.stopLogStream()
		default:
			log.Printf("gRPC: unknown ServerMessage payload type %T", p)
		}
	}

	close(results)
	<-sendDone
	return nil
}

// handleJob executes the given job and sends the result back through results.
func (c *GRPCClient) handleJob(ctx context.Context, job *pb.ExecuteJobPayload, results chan<- *pb.NodeMessage) {
	log.Printf("⚙️  gRPC job %s: executing command %q", job.GetJobId(), job.GetCommandName())

	res := c.registry.Execute(ctx, job.GetCommandName(), nil)

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

// ─── Log streaming ────────────────────────────────────────────────────────────

// startLogStream launches a goroutine that streams logs and sends each line as
// a LogEntryPayload through the results channel.
//
// If the unit value starts with the "docker:" prefix the goroutine runs
// "docker logs -f <container>" instead of journalctl.  Any other value (or an
// empty string) falls back to journalctl.
func (c *GRPCClient) startLogStream(ctx context.Context, req *pb.StreamLogsPayload, results chan<- *pb.NodeMessage) {
	c.stopLogStream() // cancel any previous stream

	logCtx, cancel := context.WithCancel(ctx)

	c.logMu.Lock()
	c.logCancel = cancel
	c.logMu.Unlock()

	unit := req.GetUnit()

	if strings.HasPrefix(unit, "docker:") {
		containerName := strings.TrimPrefix(unit, "docker:")
		go c.streamDockerLogs(logCtx, cancel, containerName, req.GetLines(), results)
	} else {
		go c.streamJournalctlLogs(logCtx, cancel, unit, req.GetLines(), results)
	}
}

// streamDockerLogs tails "docker logs -f <container>" and forwards each line.
// Both stdout and stderr of the container are merged into a single stream since
// docker logs multiplex both onto the same output.
func (c *GRPCClient) streamDockerLogs(logCtx context.Context, cancel context.CancelFunc, containerName string, histLines int32, results chan<- *pb.NodeMessage) {
	defer cancel()

	sendMsg := func(msg string) {
		select {
		case results <- &pb.NodeMessage{
			Payload: &pb.NodeMessage_LogEntry{LogEntry: &pb.LogEntryPayload{
				TimestampUs: time.Now().UnixMicro(),
				Unit:        "docker:" + containerName,
				Message:     msg,
			}},
		}:
		case <-logCtx.Done():
		}
	}

	firstRun := true
	for {
		if logCtx.Err() != nil {
			return
		}

		args := []string{"logs", "-f", "--timestamps"}
		if firstRun && histLines > 0 {
			args = append(args, "--tail", fmt.Sprintf("%d", histLines))
		} else {
			args = append(args, "--tail", "0")
		}
		args = append(args, containerName)

		cmd := exec.CommandContext(logCtx, "docker", args...)
		// Merge stdout+stderr: container log lines appear on both streams.
		pr, pw := io.Pipe()
		cmd.Stdout = pw
		cmd.Stderr = pw

		if err := cmd.Start(); err != nil {
			pw.CloseWithError(err)
			pr.Close()
			log.Printf("⚠️  docker logs start: %v", err)
			sendMsg("⚠️ docker logs start error: " + err.Error())
			return
		}
		log.Printf("📋 docker log stream started (container=%q lines=%d firstRun=%v)", containerName, histLines, firstRun)
		firstRun = false

		// Close the write end once the process finishes so the scanner exits.
		go func() {
			cmd.Wait()
			pw.Close()
		}()

		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			select {
			case results <- &pb.NodeMessage{
				Payload: &pb.NodeMessage_LogEntry{LogEntry: &pb.LogEntryPayload{
					TimestampUs: time.Now().UnixMicro(),
					Unit:        "docker:" + containerName,
					Message:     line,
				}},
			}:
			case <-logCtx.Done():
				cmd.Process.Kill()
				pr.Close()
				return
			default:
				// drop if the send buffer is full
			}
		}
		pr.Close()

		if logCtx.Err() != nil {
			log.Printf("📋 docker log stream stopped (container=%q)", containerName)
			return
		}

		log.Printf("⚠️  docker logs exited unexpectedly (container=%q), retrying in 3s…", containerName)
		sendMsg("⚠️ docker logs exited, retrying in 3s…")
		select {
		case <-logCtx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

// streamJournalctlLogs tails journalctl and forwards each line.
func (c *GRPCClient) streamJournalctlLogs(logCtx context.Context, cancel context.CancelFunc, unit string, histLines int32, results chan<- *pb.NodeMessage) {
	defer cancel()

	sendMsg := func(u, msg string) {
		select {
		case results <- &pb.NodeMessage{
			Payload: &pb.NodeMessage_LogEntry{LogEntry: &pb.LogEntryPayload{
				TimestampUs: time.Now().UnixMicro(),
				Unit:        u,
				Message:     msg,
			}},
		}:
		case <-logCtx.Done():
		}
	}

	// firstRun controls tail lines: show history on first launch, skip on retries.
	firstRun := true
	for {
		if logCtx.Err() != nil {
			return
		}

		args := []string{"-f", "--output=short-iso", "--no-pager"}
		if firstRun && histLines > 0 {
			args = append(args, "-n", fmt.Sprintf("%d", histLines))
		} else {
			args = append(args, "-n", "0")
		}
		if unit != "" {
			args = append(args, "-u", unit)
		}

		cmd := exec.CommandContext(logCtx, "journalctl", args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Printf("⚠️  journalctl pipe: %v", err)
			sendMsg("agent", "⚠️ journalctl pipe error: "+err.Error())
			return
		}
		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf

		if err := cmd.Start(); err != nil {
			log.Printf("⚠️  journalctl start: %v", err)
			sendMsg("agent", "⚠️ journalctl start error: "+err.Error())
			return
		}
		log.Printf("📋 log stream started (unit=%q lines=%d firstRun=%v)", unit, histLines, firstRun)
		firstRun = false

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			u := unit
			if u == "" {
				u = extractUnit(line)
			}
			select {
			case results <- &pb.NodeMessage{
				Payload: &pb.NodeMessage_LogEntry{LogEntry: &pb.LogEntryPayload{
					TimestampUs: time.Now().UnixMicro(),
					Unit:        u,
					Message:     line,
				}},
			}:
			case <-logCtx.Done():
				cmd.Process.Kill()
				return
			default:
				// drop if the send buffer is full
			}
		}
		cmd.Wait()

		if errOut := strings.TrimSpace(stderrBuf.String()); errOut != "" {
			log.Printf("⚠️  journalctl stderr: %s", errOut)
			sendMsg("agent", "⚠️ journalctl: "+errOut)
		}

		// If context was cancelled (stream stopped intentionally), exit cleanly.
		if logCtx.Err() != nil {
			log.Printf("📋 log stream stopped (unit=%q)", unit)
			return
		}

		// journalctl exited unexpectedly (e.g. journal not ready in container).
		// Wait briefly and retry so the UI keeps receiving logs once available.
		log.Printf("⚠️  journalctl exited unexpectedly (unit=%q), retrying in 3s…", unit)
		sendMsg("agent", "⚠️ journalctl exited, retrying in 3s…")
		select {
		case <-logCtx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

// stopLogStream cancels the active journalctl goroutine (if any).
func (c *GRPCClient) stopLogStream() {
	c.logMu.Lock()
	defer c.logMu.Unlock()
	if c.logCancel != nil {
		c.logCancel()
		c.logCancel = nil
	}
}

// extractUnit tries to parse the systemd unit name from a journalctl line.
// journalctl --output=short-iso lines look like:
//
//	2026-03-09T12:00:00+0000 hostname unit[pid]: message
func extractUnit(line string) string {
	parts := strings.Fields(line)
	if len(parts) >= 3 {
		u := parts[2]
		// Strip "[pid]:" suffix if present
		if idx := strings.IndexByte(u, '['); idx > 0 {
			return u[:idx]
		}
		if strings.HasSuffix(u, ":") {
			return u[:len(u)-1]
		}
		return u
	}
	return ""
}
