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
