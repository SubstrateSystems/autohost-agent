package commands

import (
	"context"
	"encoding/json"
	"fmt"

	"autohost-agent/internal/infra/caddy"
)

// caddyClient is a package-level default Caddy client using the standard admin URL.
// Commands read CADDY_ADMIN_URL from env via the client constructor default.
var caddyClient = caddy.NewClient("")

// CaddyUpsertRoute adds or replaces a reverse-proxy route in Caddy.
// Expected payload keys:
//   - "domain"   string — e.g. "myapp.example.com"
//   - "upstream" string — e.g. "localhost:3000"
type CaddyUpsertRoute struct{}

func (c *CaddyUpsertRoute) Execute(_ context.Context, payload map[string]any) error {
	domain, ok := payload["domain"].(string)
	if !ok || domain == "" {
		return fmt.Errorf("caddy.upsert-route: missing domain")
	}
	upstream, ok := payload["upstream"].(string)
	if !ok || upstream == "" {
		return fmt.Errorf("caddy.upsert-route: missing upstream")
	}

	if !caddyClient.IsRunning() {
		return fmt.Errorf("caddy.upsert-route: Caddy admin API not reachable at localhost:2019")
	}

	if err := caddyClient.UpsertRoute(domain, upstream); err != nil {
		return fmt.Errorf("caddy.upsert-route: %w", err)
	}
	fmt.Printf("✅ Caddy: route %s → %s configured\n", domain, upstream)
	return nil
}

// CaddyDeleteRoute removes a reverse-proxy route from Caddy.
// Expected payload keys:
//   - "domain" string
type CaddyDeleteRoute struct{}

func (c *CaddyDeleteRoute) Execute(_ context.Context, payload map[string]any) error {
	domain, ok := payload["domain"].(string)
	if !ok || domain == "" {
		return fmt.Errorf("caddy.delete-route: missing domain")
	}

	if !caddyClient.IsRunning() {
		return fmt.Errorf("caddy.delete-route: Caddy admin API not reachable at localhost:2019")
	}

	if err := caddyClient.DeleteRoute(domain); err != nil {
		return fmt.Errorf("caddy.delete-route: %w", err)
	}
	fmt.Printf("🗑️  Caddy: route %s removed\n", domain)
	return nil
}

// CaddyStatus reports whether Caddy is running and lists current routes.
// No payload required. Implements CommandWithOutput so the JSON is captured
// in the job result and visible to the frontend.
type CaddyStatus struct{}

func (c *CaddyStatus) Execute(_ context.Context, payload map[string]any) error {
	_, err := c.ExecuteWithOutput(context.Background(), payload)
	return err
}

func (c *CaddyStatus) ExecuteWithOutput(_ context.Context, _ map[string]any) (string, error) {
	if !caddyClient.IsRunning() {
		return `{"caddy_running":false}`, nil
	}

	routes, err := caddyClient.ListAllRoutes()
	if err != nil {
		return "", fmt.Errorf("caddy.status: %w", err)
	}

	out, _ := json.MarshalIndent(map[string]any{
		"caddy_running": true,
		"routes":        routes,
	}, "", "  ")
	return string(out), nil
}
