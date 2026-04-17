package agent

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
)

// UpdateTokensInConfig updates the agent_token and refresh_token in the config file.
// This is called when tokens are refreshed to persist them to disk.
func (a *Agent) UpdateTokensInConfig(configPath string) error {
	accessToken, refreshToken := a.apiClient.GetTokens()
	
	content, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsPermission(err) {
			return fmt.Errorf("error reading config: %w", err)
		}
		// File is owned by root — read it via sudo.
		out, sudoErr := exec.Command("sudo", "cat", configPath).Output()
		if sudoErr != nil {
			return fmt.Errorf("error reading config: %w", err)
		}
		content = out
	}

	updated := string(content)
	updated = regexp.MustCompile(`(?m)^agent_token:.*$`).
		ReplaceAllString(updated, fmt.Sprintf(`agent_token: "%s"`, accessToken))
	updated = regexp.MustCompile(`(?m)^refresh_token:.*$`).
		ReplaceAllString(updated, fmt.Sprintf(`refresh_token: "%s"`, refreshToken))

	if os.Geteuid() == 0 {
		return os.WriteFile(configPath, []byte(updated), 0600)
	}

	// Write to a temp file first, then copy with sudo.
	tmp, err := os.CreateTemp("", "autohost-config-update-*.yaml")
	if err != nil {
		return fmt.Errorf("error creating temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if err := os.WriteFile(tmp.Name(), []byte(updated), 0600); err != nil {
		return fmt.Errorf("error writing temp file: %w", err)
	}
	tmp.Close()

	cmd := exec.Command("sudo", "cp", tmp.Name(), configPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error copying file with sudo: %w", err)
	}
	return nil
}

// checkAndUpdateTokens checks if tokens have changed and persists them to config.
func (a *Agent) checkAndUpdateTokens() error {
	currentAccess, currentRefresh := a.apiClient.GetTokens()
	
	// Check if tokens have changed
	if currentAccess != a.lastKnownTokens.access || currentRefresh != a.lastKnownTokens.refresh {
		if err := a.UpdateTokensInConfig(a.configPath); err != nil {
			return fmt.Errorf("failed to persist updated tokens: %w", err)
		}
		
		// Update our tracking
		a.lastKnownTokens.access = currentAccess
		a.lastKnownTokens.refresh = currentRefresh
		
		log.Printf("✅ Tokens updated and persisted to config")
	}
	
	return nil
}