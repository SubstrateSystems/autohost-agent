package transport

import (
	"context"
	"strings"

	pb "autohost-agent/internal/grpc/nodepb"
)

// ─── Log streaming ────────────────────────────────────────────────────────────

// startLogStream launches a goroutine that streams logs and sends each line as
// a LogEntryPayload through the results channel.
//
// If the unit value starts with the "docker:" prefix the goroutine runs
// "docker logs -f <container>" instead of journalctl.  Any other value (or an
// empty string) falls back to journalctl.
//
// Before starting a new goroutine the previous one is cancelled and we wait
// for it to fully exit. Without this wait, rapidly toggling log streams (e.g.
// frontend reconnections) can stack up live journalctl/docker-logs processes
// and their pipe buffers, growing RAM indefinitely.
func (c *GRPCClient) startLogStream(ctx context.Context, req *pb.StreamLogsPayload, results chan<- *pb.NodeMessage) {
	// Grab cancel + done for the currently running goroutine (if any).
	c.logMu.Lock()
	prevCancel := c.logCancel
	prevDone := c.logDone
	c.logMu.Unlock()

	// Cancel the previous stream and wait for its goroutine to fully exit
	// before allocating a new context, so we never overlap two log processes.
	if prevCancel != nil {
		prevCancel()
	}
	if prevDone != nil {
		<-prevDone
	}

	logCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	c.logMu.Lock()
	c.logCancel = cancel
	c.logDone = done
	c.logMu.Unlock()

	unit := req.GetUnit()

	if strings.HasPrefix(unit, "docker:") {
		containerName := strings.TrimPrefix(unit, "docker:")
		go func() {
			defer close(done)
			c.streamDockerLogs(logCtx, cancel, containerName, req.GetLines(), results)
		}()
	} else {
		go func() {
			defer close(done)
			c.streamJournalctlLogs(logCtx, cancel, unit, req.GetLines(), results)
		}()
	}
}

// stopLogStream cancels the active log goroutine (if any).
func (c *GRPCClient) stopLogStream() {
	c.logMu.Lock()
	cancel := c.logCancel
	c.logCancel = nil
	c.logMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
