package transport

import (
	pb "autohost-agent/internal/grpc/nodepb"
	"context"
	"strings"
)

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

// stopLogStream cancels the active journalctl goroutine (if any).
func (c *GRPCClient) stopLogStream() {
	c.logMu.Lock()
	defer c.logMu.Unlock()
	if c.logCancel != nil {
		c.logCancel()
		c.logCancel = nil
	}
}
