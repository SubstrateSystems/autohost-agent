package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"autohost-agent/internal/domain"
	"autohost-agent/pkg/shell"
)

// ScriptExec executes a custom command script received via a job.
// Expected payload keys:
//   - "script_path": absolute path to the .sh file
type ScriptExec struct{}

func (c *ScriptExec) Execute(_ context.Context, payload map[string]any) error {
	scriptPath, ok := payload["script_path"].(string)
	if !ok || scriptPath == "" {
		return fmt.Errorf("missing or invalid script_path in payload")
	}

	// Validate the script name component.
	name := domain.ScriptName(filepath.Base(scriptPath))
	if err := name.Validate(); err != nil {
		return fmt.Errorf("invalid script: %w", err)
	}

	// Ensure the script lives inside the allowed commands directory.
	absPath, err := filepath.Abs(scriptPath)
	if err != nil {
		return fmt.Errorf("cannot resolve script path: %w", err)
	}

	allowedDir, err := filepath.Abs(domain.CustomCommandsDir)
	if err != nil {
		return fmt.Errorf("cannot resolve commands dir: %w", err)
	}

	rel, err := filepath.Rel(allowedDir, absPath)
	if err != nil || len(rel) > 1 && rel[:2] == ".." {
		return fmt.Errorf("script must be inside %s", domain.CustomCommandsDir)
	}

	// Verify the file exists and is executable.
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("script not found: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("script_path points to a directory, not a file")
	}

	fmt.Printf("🔧 Ejecutando custom command: %s\n", filepath.Base(absPath))
	if err := shell.Exec("bash", absPath); err != nil {
		return fmt.Errorf("custom command failed: %w", err)
	}
	fmt.Printf("✅ Custom command '%s' completado.\n", filepath.Base(absPath))
	return nil
}
