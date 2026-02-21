package docker

import "os/exec"

func DockerInstalled() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}
