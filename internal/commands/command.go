package commands

import (
	"context"
	"fmt"
)

// Command is the interface that all agent commands must implement.
type Command interface {
	// Execute runs the command with the given context and payload.
	Execute(ctx context.Context, payload map[string]any) error
}

// Registry maps job types to their command handlers.
type Registry struct {
	handlers map[string]Command
}

// NewRegistry creates a new command registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]Command),
	}
}

// Register adds a command handler for the given job type.
func (r *Registry) Register(jobType string, cmd Command) {
	r.handlers[jobType] = cmd
}

// Execute finds and executes the command for the given job type.
func (r *Registry) Execute(ctx context.Context, jobType string, payload map[string]any) error {
	cmd, ok := r.handlers[jobType]
	if !ok {
		return fmt.Errorf("unknown command type: %s", jobType)
	}
	return cmd.Execute(ctx, payload)
}

// RegisterAll registers all built-in commands in the registry.
// Call this once during agent startup.
func RegisterAll(r *Registry) {
	r.Register("app.start", &AppStart{})
	r.Register("app.stop", &AppStop{})
	r.Register("app.restart", &AppRestart{})
	r.Register("app.remove", &AppRemove{})
	r.Register("docker.install", &DockerInstall{})
}
