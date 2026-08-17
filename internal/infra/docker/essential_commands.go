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

func Stop(appName domain.AppName) error {
	ymlPath := filepath.Join(dir.GetSubdir("apps"), string(appName), "compose.yml")

	return shell.ExecWithDir(filepath.Dir(ymlPath), "docker", "compose", "stop")
}

func Start(appName domain.AppName) error {
	appDir := filepath.Join(dir.GetSubdir("apps"), string(appName))
	ymlPath := filepath.Join(appDir, "compose.yml")

	// Validar si existe el archivo compose.yml
	content, err := os.ReadFile(ymlPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("el archivo de configuración no existe: %s", ymlPath)
	} else if err != nil {
		return fmt.Errorf("error al leer compose.yml: %w", err)
	}

	// Validar seguridad del compose antes de levantar contenedores
	if err := ValidateCompose(string(content), appDir); err != nil {
		return fmt.Errorf("validación de seguridad fallida para %s: %w", appName, err)
	}

	fmt.Printf("🔄 Levantando aplicación '%s'...\n", appName)
	// Run from the app dir — compose.yml is auto-discovered, no -f needed.
	output, err := shell.ExecWithDirOutput(appDir, "docker", "compose", "up", "-d")
	if err != nil {
		return fmt.Errorf("exit status: %w\n%s", err, output)
	}
	return nil
}

func Remove(appName domain.AppName) error {
	if err := appName.Validate(); err != nil {
		return err
	}

	appDir := filepath.Join(dir.GetSubdir("apps"), string(appName))

	if err := shell.ExecWithDir(appDir, "docker", "compose", "down"); err != nil {
		return fmt.Errorf("failed to stop app: %w", err)
	}

	return shell.Exec("rm", "-rf", appDir)
}

func ListContainers() error {
	cmd := exec.Command("docker", "ps", "--format", "{{.Names}}\t{{.Status}}")
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	fmt.Println("Contenedores en ejecución:")
	fmt.Println(string(out))
	return nil
}

func GetAppStatus(appName domain.AppName) (string, error) {
	ymlPath := filepath.Join(dir.GetSubdir("apps"), string(appName), "compose.yml")

	cmd := exec.Command("docker", "compose", "ps", "--status=running")
	cmd.Dir = filepath.Dir(ymlPath)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	if strings.Contains(string(out), "Up") {
		return "en ejecución", nil
	}
	return "detenida", nil
}
