package app

import (
	"autohost-agent/internal/adapters/docker"
	"autohost-agent/internal/domain"
	"fmt"
)

func Start(appName domain.AppName) error {
	fmt.Printf("🔄 Iniciando aplicación '%s'...\n", appName)
	if err := docker.Start(appName); err != nil {
		return err
	}
	fmt.Printf("✅ Aplicación '%s' iniciada.\n", appName)
	return nil
}
