package transport

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"autohost-agent/internal/commands"
	pb "autohost-agent/internal/grpc/nodepb"
)


type GRPCClient struct {
	address  string
	token    string
	nodeID   string
	registry *commands.Registry

	logMu     sync.Mutex
	logCancel context.CancelFunc // non-nil while log streaming is active
}

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
