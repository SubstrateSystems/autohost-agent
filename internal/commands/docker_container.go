package commands

import (
	"bytes"
	"context"
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
//	container.list                → docker ps -a (JSON array output)
//	container.start.<name>        → docker start <name>
//	container.stop.<name>         → docker stop <name>
//	container.restart.<name>      → docker restart <name>
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

	default:
		return ExecuteResult{Err: fmt.Errorf("unknown container action %q in %q", action, jobType)}
	}
}

// containerList runs docker ps -a and returns a JSON array where each element
// has the fields: id, name, image, state, status.
func containerList(ctx context.Context) ExecuteResult {
	cmd := exec.CommandContext(ctx, "docker", "ps", "-a", "--format",
		`{"id":"{{.ID}}","name":"{{.Names}}","image":"{{.Image}}","state":"{{.State}}","status":"{{.Status}}"}`)

	out, err := cmd.Output()
	if err != nil {
		return ExecuteResult{Err: fmt.Errorf("docker ps: %w", err)}
	}

	// Each output line is a self-contained JSON object; combine into an array.
	var items []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			items = append(items, line)
		}
	}

	if len(items) == 0 {
		return ExecuteResult{Output: "[]"}
	}
	return ExecuteResult{Output: "[" + strings.Join(items, ",") + "]"}
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
