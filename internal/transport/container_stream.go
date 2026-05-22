package transport

import (
	"bufio"
	"context"
	"log"
	"os/exec"
	"time"

	"autohost-agent/internal/commands"
	pb "autohost-agent/internal/grpc/nodepb"
)

// ─── Container streaming ──────────────────────────────────────────────────────

// startContainerStream launches a goroutine that sends an initial
// ContainerListPayload snapshot and then watches `docker events` to resend on
// every container lifecycle change.  Any previous stream is stopped first.
func (c *GRPCClient) startContainerStream(ctx context.Context, results chan<- *pb.NodeMessage) {
	c.stopContainerStream() // cancel any running stream

	streamCtx, cancel := context.WithCancel(ctx)

	c.containerMu.Lock()
	c.containerCancel = cancel
	c.containerMu.Unlock()

	go c.runContainerStream(streamCtx, cancel, results)
}

// stopContainerStream cancels the active container-streaming goroutine if any.
func (c *GRPCClient) stopContainerStream() {
	c.containerMu.Lock()
	defer c.containerMu.Unlock()
	if c.containerCancel != nil {
		c.containerCancel()
		c.containerCancel = nil
	}
}

// runContainerStream is the goroutine body for container streaming.
// It sends the initial snapshot, then subscribes to `docker events` and
// re-sends a fresh snapshot after each event (debounced by 500 ms to handle
// rapid sequences like stop+destroy).
func (c *GRPCClient) runContainerStream(ctx context.Context, cancel context.CancelFunc, out chan<- *pb.NodeMessage) {
	defer cancel()

	// Initial snapshot.
	c.sendContainerSnapshot(ctx, out)

	eventsCmd := exec.CommandContext(ctx, "docker", "events",
		"--filter", "type=container",
		"--filter", "event=start",
		"--filter", "event=die",
		"--filter", "event=stop",
		"--filter", "event=pause",
		"--filter", "event=unpause",
		"--filter", "event=create",
		"--filter", "event=destroy",
		"--filter", "event=rename",
		"--format", "{{.Status}}",
	)

	stdout, err := eventsCmd.StdoutPipe()
	if err != nil {
		log.Printf("⚠️  container stream: stdout pipe: %v", err)
		return
	}
	if err := eventsCmd.Start(); err != nil {
		log.Printf("⚠️  container stream: docker events start: %v", err)
		return
	}
	defer eventsCmd.Wait() //nolint:errcheck

	// eventSig receives a signal (capped at 1) whenever the scanner reads a line.
	eventSig := make(chan struct{}, 1)

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case eventSig <- struct{}{}:
			default:
				// already has a pending signal — skip
			}
		}
		close(eventSig)
	}()

	const debounce = 500 * time.Millisecond
	debounceTimer := time.NewTimer(debounce)
	debounceTimer.Stop() // starts paused
	defer debounceTimer.Stop()

	pending := false

	for {
		select {
		case <-ctx.Done():
			return

		case _, ok := <-eventSig:
			if !ok {
				// docker events exited (context cancelled or daemon restarted).
				return
			}
			if !pending {
				pending = true
				debounceTimer.Reset(debounce)
			}

		case <-debounceTimer.C:
			pending = false
			c.sendContainerSnapshot(ctx, out)
		}
	}
}

// sendContainerSnapshot calls ContainerListSnapshot and pushes the result to out.
func (c *GRPCClient) sendContainerSnapshot(ctx context.Context, out chan<- *pb.NodeMessage) {
	result := commands.ContainerListSnapshot(ctx)

	payload := &pb.ContainerListPayload{}
	if result.Err != nil {
		payload.Error = result.Err.Error()
	}
	for _, ci := range result.Containers {
		payload.Containers = append(payload.Containers, &pb.ContainerInfo{
			Id:      ci.ID,
			Name:    ci.Name,
			Image:   ci.Image,
			State:   ci.State,
			Status:  ci.Status,
			Cpu:     ci.CPU,
			Mem:     ci.Mem,
			MemPerc: ci.MemPerc,
		})
	}

	msg := &pb.NodeMessage{
		Payload: &pb.NodeMessage_ContainerList{
			ContainerList: payload,
		},
	}
	select {
	case out <- msg:
	case <-ctx.Done():
	default:
		log.Printf("⚠️  gRPC: container snapshot buffer full, skipping")
	}
}
