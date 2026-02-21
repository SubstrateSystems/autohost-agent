package commands

import (
	"context"
	"fmt"

	"autohost-agent/internal/infra/docker"
)

// AppStop stops a docker compose application.
type AppStop struct{}

func (c *AppStop) Execute(_ context.Context, payload map[string]any) error {
	appName, err := extractAppName(payload)
	if err != nil {
		return err
	}
	fmt.Printf("🛑 Deteniendo aplicación '%s'...\n", appName)
	if err := docker.Stop(appName); err != nil {
		return err
	}
	fmt.Printf("✅ Aplicación '%s' detenida.\n", appName)
	return nil
}
