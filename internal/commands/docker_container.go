package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// safeContainerName only allows characters valid in Docker container names,
// preventing path traversal or shell metacharacter injection.
var safeContainerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_\-.]{0,127}$`)

// execContainerCommand handles container.* job types dispatched from the dashboard.
//
// Supported patterns:
//
//	container.list                  → docker ps -a with live stats (JSON array)
//	container.start.<name>          → docker start <name>
//	container.stop.<name>           → docker stop <name>
//	container.restart.<name>        → docker restart <name>
//	container.stats.<name>          → docker stats --no-stream <name> (JSON)
func execContainerCommand(ctx context.Context, jobType string) ExecuteResult {
	// jobType examples: "container.list", "container.restart.my-app"
	parts := strings.SplitN(jobType, ".", 3)
	if len(parts) < 2 || parts[0] != "container" {
		return ExecuteResult{Err: fmt.Errorf("invalid container command: %q", jobType)}
	}

	action := parts[1]
	switch action {
	case "list":
		return containerList(ctx)

	case "start", "stop", "restart":
		if len(parts) < 3 || parts[2] == "" {
			return ExecuteResult{Err: fmt.Errorf("container %s: missing container name", action)}
		}
		name := parts[2]
		if !safeContainerName.MatchString(name) {
			return ExecuteResult{Err: fmt.Errorf("container %s: invalid container name %q", action, name)}
		}
		return containerAction(ctx, action, name)

	case "stats":
		if len(parts) < 3 || parts[2] == "" {
			return ExecuteResult{Err: fmt.Errorf("container stats: missing container name")}
		}
		name := parts[2]
		if !safeContainerName.MatchString(name) {
			return ExecuteResult{Err: fmt.Errorf("container stats: invalid container name %q", name)}
		}
		return containerStats(ctx, name)

	case "logs":
		if len(parts) < 3 || parts[2] == "" {
			return ExecuteResult{Err: fmt.Errorf("container logs: missing container name")}
		}
		name := parts[2]
		if !safeContainerName.MatchString(name) {
			return ExecuteResult{Err: fmt.Errorf("container logs: invalid container name %q", name)}
		}
		cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", "100", name)
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		if err := cmd.Run(); err != nil {
			return ExecuteResult{Output: buf.String(), Err: fmt.Errorf("docker logs %s: %w", name, err)}
		}
		return ExecuteResult{Output: buf.String()}

	default:
		return ExecuteResult{Err: fmt.Errorf("unknown container action %q in %q", action, jobType)}
	}
}

// containerInfo is the JSON shape returned for each container in container.list.
type containerInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Status  string `json:"status"`
	CPU     string `json:"cpu,omitempty"`
	Mem     string `json:"mem,omitempty"`
	MemPerc string `json:"memPerc,omitempty"`
}

// ContainerEntry is the public version of containerInfo used by the transport
// layer to construct proto messages without going through JSON marshaling.
type ContainerEntry struct {
	ID      string
	Name    string
	Image   string
	State   string
	Status  string
	CPU     string
	Mem     string
	MemPerc string
}

// ContainerSnapshotResult is returned by ContainerListSnapshot.
type ContainerSnapshotResult struct {
	Containers []ContainerEntry
	Err        error
}

// ContainerListSnapshot returns the current container list as a Go struct
// suitable for direct proto conversion, without JSON marshaling.
func ContainerListSnapshot(ctx context.Context) ContainerSnapshotResult {
	raw, err := containerListRaw(ctx)
	if err != nil {
		return ContainerSnapshotResult{Err: err}
	}
	entries := make([]ContainerEntry, 0, len(raw))
	for _, c := range raw {
		entries = append(entries, ContainerEntry{
			ID:      c.ID,
			Name:    c.Name,
			Image:   c.Image,
			State:   c.State,
			Status:  c.Status,
			CPU:     c.CPU,
			Mem:     c.Mem,
			MemPerc: c.MemPerc,
		})
	}
	return ContainerSnapshotResult{Containers: entries}
}

// rawStats is used to parse the JSON output of docker stats.
type rawStats struct {
	Name    string `json:"name"`
	CPU     string `json:"cpu"`
	Mem     string `json:"mem"`
	MemPerc string `json:"memPerc"`
}

// containerListRaw runs docker ps + docker stats and returns the raw struct
// slice.  It is the shared core used by both containerList (JSON output for
// jobs) and ContainerListSnapshot (struct output for gRPC streaming).
func containerListRaw(ctx context.Context) ([]containerInfo, error) {
	psCmd := exec.CommandContext(ctx, "docker", "ps", "-a",
		"--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.State}}\t{{.Status}}")
	psOut, err := psCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}

	statsMap := map[string]rawStats{}
	statsCmd := exec.CommandContext(ctx, "docker", "stats", "--no-stream",
		"--format", `{"name":"{{.Name}}","cpu":"{{.CPUPerc}}","mem":"{{.MemUsage}}","memPerc":"{{.MemPerc}}"}`)
	if statsOut, statsErr := statsCmd.Output(); statsErr == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(statsOut)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var s rawStats
			if jsonErr := json.Unmarshal([]byte(line), &s); jsonErr == nil && s.Name != "" {
				statsMap[strings.TrimPrefix(s.Name, "/")] = s
			}
		}
	}

	var result []containerInfo
	for _, line := range strings.Split(strings.TrimSpace(string(psOut)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 5)
		if len(parts) != 5 {
			continue
		}
		c := containerInfo{
			ID:     parts[0],
			Name:   parts[1],
			Image:  parts[2],
			State:  parts[3],
			Status: parts[4],
		}
		if s, ok := statsMap[parts[1]]; ok {
			c.CPU = s.CPU
			c.Mem = s.Mem
			c.MemPerc = s.MemPerc
		}
		result = append(result, c)
	}
	return result, nil
}

// containerList runs docker ps -a and merges live stats (cpu, mem, memPerc)
// for running containers via docker stats --no-stream. Returns a JSON array.
func containerList(ctx context.Context) ExecuteResult {
	result, err := containerListRaw(ctx)
	if err != nil {
		return ExecuteResult{Err: err}
	}

	if len(result) == 0 {
		return ExecuteResult{Output: "[]"}
	}
	b, err := json.Marshal(result)
	if err != nil {
		return ExecuteResult{Err: fmt.Errorf("marshal containers: %w", err)}
	}
	return ExecuteResult{Output: string(b)}
}

// containerAction runs docker start/stop/restart <name>.
// exec.Command is used directly with individual args — not a shell — so the
// container name cannot be used to inject additional shell commands.
func containerAction(ctx context.Context, action, name string) ExecuteResult {
	cmd := exec.CommandContext(ctx, "docker", action, name)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return ExecuteResult{Output: buf.String(), Err: fmt.Errorf("docker %s %s: %w", action, name, err)}
	}
	return ExecuteResult{Output: buf.String()}
}

func containerStats(ctx context.Context, name string) ExecuteResult {
	cmd := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format",
		`{"name":"{{.Name}}","cpu":"{{.CPUPerc}}","mem":"{{.MemUsage}}"}`, name)
	out, err := cmd.Output()
	if err != nil {
		return ExecuteResult{Err: fmt.Errorf("docker stats %s: %w", name, err)}
	}
	return ExecuteResult{Output: string(out)}
}
