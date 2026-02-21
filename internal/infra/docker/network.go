package docker

import (
	"fmt"
	"os/exec"
)

func CreateDockerNetwork() error {
	cmd := exec.Command("sudo", "docker", "network", "inspect", "autohost_net")
	if err := cmd.Run(); err == nil {
		fmt.Println("✅ La red 'autohost_net' ya existe.")
		return nil
	}
	cmd = exec.Command("sudo", "docker", "network", "create", "autohost_net")
	return cmd.Run()
}
