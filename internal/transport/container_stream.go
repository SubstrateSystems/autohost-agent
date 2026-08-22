package transport

import (
	"bufio"
	"context"
	"log"
	"os/exec"
	"strings"
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
// It sends an initial snapshot then keeps pushing updates:
//   - every periodicInterval (ticker) to refresh CPU/mem stats, and
//   - whenever a Docker lifecycle event occurs (start, stop, die, …).
//
// The docker events watcher goroutine (watchDockerEvents) restarts the
// process automatically on unexpected exits, so this loop never terminates
// due to a transient docker events failure.
func (c *GRPCClient) runContainerStream(ctx context.Context, cancel context.CancelFunc, out chan<- *pb.NodeMessage) {
	defer cancel()

	// Initial snapshot.
	c.sendContainerSnapshot(ctx, out)

	const debounce = 500 * time.Millisecond
	debounceTimer := time.NewTimer(debounce)
	debounceTimer.Stop() // starts paused
	defer debounceTimer.Stop()

	// Periodic ticker so CPU/mem stats refresh even when no lifecycle events occur.
	const periodicInterval = 10 * time.Second
	ticker := time.NewTicker(periodicInterval)
	defer ticker.Stop()

	pending := false

	// eventSig is written to by watchDockerEvents. It is NEVER closed,
	// so the select loop never sees a !ok case and never exits due to
	// docker events exiting unexpectedly.
	eventSig := make(chan struct{}, 1)
	go c.watchDockerEvents(ctx, eventSig)

	for {
		select {
		case <-ctx.Done():
			return

		case <-eventSig:
			if !pending {
				pending = true
				debounceTimer.Reset(debounce)
			}

		case <-debounceTimer.C:
			pending = false
			c.sendContainerSnapshot(ctx, out)

		case <-ticker.C:
			c.sendContainerSnapshot(ctx, out)
		}
	}
}

// watchDockerEvents runs `docker events` in a loop, writing to sig on every
// container lifecycle event. It restarts the process with exponential back-off
// whenever it exits unexpectedly. The goroutine exits only when ctx is done.
func (c *GRPCClient) watchDockerEvents(ctx context.Context, sig chan<- struct{}) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}

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
			"--format", "{{.Action}}",
		)

		var stderrBuf strings.Builder
		stdout, err := eventsCmd.StdoutPipe()
		if err != nil {
			log.Printf("⚠️  container stream: stdout pipe: %v", err)
			if !sleepWithBackoff(ctx, &backoff, maxBackoff) {
				return
			}
			continue
		}
		eventsCmd.Stderr = &stderrBuf

		if err := eventsCmd.Start(); err != nil {
			log.Printf("⚠️  container stream: docker events start: %v", err)
			if !sleepWithBackoff(ctx, &backoff, maxBackoff) {
				return
			}
			continue
		}

		scanner := bufio.NewScanner(stdout)
		scannedAny := false
		for scanner.Scan() {
			scannedAny = true
			select {
			case sig <- struct{}{}:
			default: // already has a pending signal — skip
			}
		}
		waitErr := eventsCmd.Wait()

		if ctx.Err() != nil {
			return
		}

		if scannedAny {
			backoff = time.Second // reset backoff if it had successfully received events
		}

		errDetail := strings.TrimSpace(stderrBuf.String())
		if errDetail != "" {
			log.Printf("⚠️  container stream: docker events exited: %s (err: %v)", errDetail, waitErr)
		} else if waitErr != nil {
			log.Printf("⚠️  container stream: docker events exited unexpectedly (err: %v)", waitErr)
		} else {
			log.Printf("⚠️  container stream: docker events exited unexpectedly, restarting")
		}

		if !sleepWithBackoff(ctx, &backoff, maxBackoff) {
			return
		}
	}
}

// sleepWithBackoff waits for the current backoff duration (respecting ctx
// cancellation), then doubles it up to maxBackoff. Returns false if ctx was
// cancelled during the wait.
func sleepWithBackoff(ctx context.Context, backoff *time.Duration, maxBackoff time.Duration) bool {
	select {
	case <-time.After(*backoff):
	case <-ctx.Done():
		return false
	}
	*backoff *= 2
	if *backoff > maxBackoff {
		*backoff = maxBackoff
	}
	return true
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
