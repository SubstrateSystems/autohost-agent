package docker

import (
	"autohost-agent/internal/domain"
	"autohost-agent/pkg/dir"
	"autohost-agent/pkg/shell"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func StopApp(app domain.AppName) error {
	ymlPath := filepath.Join(dir.GetSubdir("apps"), string(app), "compose.yml")

	return shell.ExecWithDir(filepath.Dir(ymlPath), "docker", "compose", "-f", ymlPath, "stop")
}

func StartApp(app domain.AppName) error {
	ymlPath := filepath.Join(dir.GetSubdir("apps"), string(app), "compose.yml")

	// Validar si existe el archivo compose.yml
	if _, err := os.Stat(ymlPath); os.IsNotExist(err) {
		return fmt.Errorf("el archivo de configuración no existe: %s", ymlPath)
	}

	fmt.Printf("🔄 Levantando aplicación '%s'...\n", app)

	// Usar Exec con working dir del compose
	return shell.ExecWithDir(filepath.Dir(ymlPath), "docker", "compose", "-f", ymlPath, "up", "-d")
}

func RemoveApp(app domain.AppName) error {
	if err := app.Validate(); err != nil {
		return err
	}

	appDir := filepath.Join(dir.GetSubdir("apps"), string(app))
	ymlPath := filepath.Join(appDir, "compose.yml")

	if err := shell.ExecWithDir(appDir, "docker", "compose", "-f", ymlPath, "down"); err != nil {
		return fmt.Errorf("failed to stop app: %w", err)
	}

	return shell.Exec("rm", "-rf", appDir)
}

func GetAppStatus(app domain.AppName) (string, error) {
	ymlPath := filepath.Join(dir.GetSubdir("apps"), string(app), "compose.yml")

	cmd := exec.Command("docker", "compose", "-f", ymlPath, "ps", "--status=running")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	if strings.Contains(string(out), "Up") {
		return "en ejecución", nil
	}
	return "detenida", nil
}
