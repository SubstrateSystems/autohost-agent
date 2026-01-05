package app

import (
	"autohost-agent/internal/adapters/docker"
	"autohost-agent/internal/domain"
	"fmt"
)

func Restart(appName domain.AppName) error {
	fmt.Printf("🔄 Reiniciando aplicación '%s'...\n", appName)
	if err := docker.Stop(appName); err != nil {
		return err
	}
	if err := docker.Start(appName); err != nil {
		return err
	}
	fmt.Printf("✅ Aplicación '%s' reiniciada.\n", appName)
	return nil
}
