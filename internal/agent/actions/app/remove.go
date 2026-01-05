package app

import (
	"autohost-agent/internal/adapters/docker"
	"autohost-agent/internal/domain"
	"fmt"
)

func Remove(appName domain.AppName) error {
	fmt.Printf("🗑️  Eliminando aplicación '%s'...\n", appName)
	if err := docker.Remove(appName); err != nil {
		return err
	}
	fmt.Printf("✅ Aplicación '%s' eliminada.\n", appName)
	return nil
}
