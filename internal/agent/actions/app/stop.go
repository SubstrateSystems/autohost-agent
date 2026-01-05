package app

import (
	"autohost-agent/internal/adapters/docker"
	"autohost-agent/internal/domain"
	"fmt"
)

func Stop(appName domain.AppName) error {
	fmt.Printf("🛑 Deteniendo aplicación '%s'...\n", appName)
	if err := docker.Stop(appName); err != nil {
		return err
	}
	fmt.Printf("✅ Aplicación '%s' detenida.\n", appName)
	return nil
}
