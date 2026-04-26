package caddy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultAdminURL = "http://localhost:2019"

// routeIDPrefix is prepended to every domain to namespace autohost-managed routes.
const routeIDPrefix = "autohost-"

// Client talks to the Caddy admin API.
type Client struct {
	adminURL   string
	httpClient *http.Client
}

// NewClient creates a Caddy admin API client.
// adminURL defaults to http://localhost:2019 when empty.
func NewClient(adminURL string) *Client {
	if adminURL == "" {
		adminURL = defaultAdminURL
	}
	return &Client{
		adminURL:   strings.TrimRight(adminURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// IsRunning returns true if the Caddy admin API is reachable.
func (c *Client) IsRunning() bool {
	resp, err := c.httpClient.Get(c.adminURL + "/config/")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// caddyRoute represents a single Caddy HTTP route object.
type caddyRoute struct {
	ID       string        `json:"@id"`
	Match    []matchBlock  `json:"match"`
	Handle   []handleBlock `json:"handle"`
	Terminal bool          `json:"terminal"`
}

type matchBlock struct {
	Host []string `json:"host"`
}

type handleBlock struct {
	Handler   string       `json:"handler"`
	Upstreams []upstream   `json:"upstreams,omitempty"`
	Routes    []caddyRoute `json:"routes,omitempty"`
}

type upstream struct {
	Dial string `json:"dial"`
}

// Route is a simplified view of a Caddy reverse-proxy route.
type Route struct {
	Domain   string `json:"domain"`
	Upstream string `json:"upstream"`
}

func routeID(domain string) string {
	return routeIDPrefix + strings.ReplaceAll(domain, ".", "-")
}

func buildRoute(domain, upstreamDial string) caddyRoute {
	return caddyRoute{
		ID:       routeID(domain),
		Match:    []matchBlock{{Host: []string{domain}}},
		Handle:   []handleBlock{{Handler: "reverse_proxy", Upstreams: []upstream{{Dial: upstreamDial}}}},
		Terminal: true,
	}
}

// UpsertRoute creates or replaces a reverse-proxy route for the given domain.
// upstreamDial is e.g. "localhost:3000".
func (c *Client) UpsertRoute(domain, upstreamDial string) error {
	id := routeID(domain)
	route := buildRoute(domain, upstreamDial)

	body, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("marshal route: %w", err)
	}

	// Try PUT /id/{id} first (updates existing named block).
	if err := c.putID(id, body); err == nil {
		return nil
	}

	// If the named block doesn't exist yet, append to the server routes list.
	// Ensure the HTTP server exists first.
	if err := c.ensureServer(); err != nil {
		return fmt.Errorf("ensure server: %w", err)
	}

	return c.postJSON("/config/apps/http/servers/autohost/routes", body)
}

// DeleteRoute removes the reverse-proxy route for the given domain.
func (c *Client) DeleteRoute(domain string) error {
	id := routeID(domain)
	req, err := http.NewRequest(http.MethodDelete, c.adminURL+"/id/"+id, nil)
	if err != nil {
		return fmt.Errorf("build delete request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete /id/%s: %w", id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil // already gone — idempotent
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete route %s: status %d: %s", domain, resp.StatusCode, string(b))
	}
	return nil
}

// ListRoutes returns all autohost-managed routes currently in Caddy.
func (c *Client) ListRoutes() ([]Route, error) {
	resp, err := c.httpClient.Get(c.adminURL + "/config/apps/http/servers/autohost/routes")
	if err != nil {
		return nil, fmt.Errorf("get routes: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return []Route{}, nil
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list routes: status %d", resp.StatusCode)
	}

	var routes []caddyRoute
	if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		return nil, fmt.Errorf("decode routes: %w", err)
	}

	var out []Route
	for _, r := range routes {
		if !strings.HasPrefix(r.ID, routeIDPrefix) {
			continue // skip routes not managed by autohost
		}
		if len(r.Match) == 0 || len(r.Match[0].Host) == 0 {
			continue
		}
		domain := r.Match[0].Host[0]
		dial := ""
		if len(r.Handle) > 0 && len(r.Handle[0].Upstreams) > 0 {
			dial = r.Handle[0].Upstreams[0].Dial
		}
		out = append(out, Route{Domain: domain, Upstream: dial})
	}
	return out, nil
}

// putID sends a PUT to /id/{id} with the given JSON body.
func (c *Client) putID(id string, body []byte) error {
	req, err := http.NewRequest(http.MethodPut, c.adminURL+"/id/"+id, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("id %s not found", id)
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("put id %s: status %d: %s", id, resp.StatusCode, string(b))
	}
	return nil
}

// postJSON sends a POST with the given JSON body to path.
func (c *Client) postJSON(path string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, c.adminURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("post %s: status %d: %s", path, resp.StatusCode, string(b))
	}
	return nil
}

// ensureServer makes sure the Caddy config has an HTTP server named "autohost"
// with an HTTPS listener. It is safe to call multiple times.
func (c *Client) ensureServer() error {
	resp, err := c.httpClient.Get(c.adminURL + "/config/apps/http/servers/autohost")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil // already exists
	}

	// Create the server with an HTTPS listen address and auto HTTPS.
	server := map[string]any{
		"listen": []string{":443", ":80"},
		"routes": []any{},
	}
	body, _ := json.Marshal(server)
	return c.postJSON("/config/apps/http/servers/autohost", body)
}
