package transport

import (
	pb "autohost-agent/internal/grpc/nodepb"
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"time"
)

// streamDockerLogs tails "docker logs -f <container>" and forwards each line.
// Both stdout and stderr of the container are merged into a single stream since
// docker logs multiplex both onto the same output

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

		// iterCtx is cancelled when either this iteration ends or the outer
		// logCtx is cancelled. Using a per-iteration context prevents goroutine
		// accumulation across retries (e.g. when a container repeatedly restarts).
		iterCtx, iterCancel := context.WithCancel(logCtx)

		// Close the write end once the process finishes so the scanner exits.
		// Also cancel iterCtx so the pr-close goroutine below exits promptly.
		go func() {
			cmd.Wait()
			pw.Close()
			iterCancel()
		}()

		// Guarantee scanner.Scan() unblocks quickly when this iteration ends:
		// close the read end of the pipe directly so that the blocking Scan()
		// call returns EOF without waiting for the process kill → Wait → pw.Close() chain.
		go func() {
			<-iterCtx.Done()
			pr.Close()
		}()

		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			line = sanitizeLogLine(line)
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
		iterCancel() // ensure the pr-close goroutine exits before next iteration

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
