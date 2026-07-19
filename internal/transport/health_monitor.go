package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	pb "autohost-agent/internal/grpc/nodepb"
)

// healthCheckConfig mirrors the proto payload for local use.
type healthCheckConfig struct {
	MonitorID          string
	ServiceName        string
	CheckType          string // "process" | "tcp" | "http"
	HTTPURL            string
	HTTPExpectedStatus int
	TCPHost            string
	TCPPort            int
	IntervalSeconds    int
	FailureThreshold   int
	Enabled            bool
}

// healthMonitor manages all active health check loops for a single agent session.
type healthMonitor struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc // monitorID -> cancel
}

func newHealthMonitor() *healthMonitor {
	return &healthMonitor{cancels: make(map[string]context.CancelFunc)}
}

// upsert starts or restarts the health check loop for the given config.
func (hm *healthMonitor) upsert(ctx context.Context, cfg healthCheckConfig, results chan<- *pb.NodeMessage) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	// Cancel existing loop for this monitor, if any.
	if cancel, ok := hm.cancels[cfg.MonitorID]; ok {
		cancel()
	}

	if !cfg.Enabled {
		delete(hm.cancels, cfg.MonitorID)
		log.Printf("🔍 health-check %s (%s): disabled", cfg.ServiceName, cfg.MonitorID)
		return
	}

	loopCtx, cancel := context.WithCancel(ctx)
	hm.cancels[cfg.MonitorID] = cancel

	interval := time.Duration(cfg.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}

	go func() {
		log.Printf("🔍 health-check %s (%s): started every %s", cfg.ServiceName, cfg.MonitorID, interval)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		consecutiveFailures := 0

		for {
			select {
			case <-loopCtx.Done():
				log.Printf("🔍 health-check %s (%s): stopped", cfg.ServiceName, cfg.MonitorID)
				return
			case <-ticker.C:
				status, latencyMs, msg := runCheck(loopCtx, cfg)
				if status == "up" {
					consecutiveFailures = 0
				} else {
					consecutiveFailures++
				}

				payload := &pb.NodeMessage{
					Payload: &pb.NodeMessage_HealthResult{
						HealthResult: &pb.HealthCheckResultPayload{
							MonitorId:           cfg.MonitorID,
							Status:              status,
							LatencyMs:           int32(latencyMs),
							Message:             msg,
							ConsecutiveFailures: int32(consecutiveFailures),
						},
					},
				}
				select {
				case results <- payload:
				case <-loopCtx.Done():
					return
				}
			}
		}
	}()
}

// remove stops the health check loop for the given monitor ID.
func (hm *healthMonitor) remove(monitorID string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	if cancel, ok := hm.cancels[monitorID]; ok {
		cancel()
		delete(hm.cancels, monitorID)
	}
}

// stopAll stops all running health check loops.
func (hm *healthMonitor) stopAll() {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	for id, cancel := range hm.cancels {
		cancel()
		delete(hm.cancels, id)
	}
}

// ─── Check implementations ───────────────────────────────────────────────────

// runCheck performs a single health check and returns (status, latencyMs, message).
func runCheck(ctx context.Context, cfg healthCheckConfig) (string, int, string) {
	start := time.Now()

	switch cfg.CheckType {
	case "http":
		return checkHTTP(ctx, cfg, start)
	case "tcp":
		return checkTCP(ctx, cfg, start)
	default: // "process" — check Docker container status
		return checkProcess(ctx, cfg, start)
	}
}

func checkHTTP(ctx context.Context, cfg healthCheckConfig, start time.Time) (string, int, string) {
	if cfg.HTTPURL == "" {
		return "down", 0, "http_url not configured"
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, cfg.HTTPURL, nil)
	if err != nil {
		return "down", 0, fmt.Sprintf("build request: %v", err)
	}
	req.Header.Set("User-Agent", "autohost-agent/healthcheck")

	resp, err := http.DefaultClient.Do(req)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return "down", latencyMs, fmt.Sprintf("http error: %v", err)
	}
	resp.Body.Close()

	expected := cfg.HTTPExpectedStatus
	if expected == 0 {
		expected = 200
	}
	if resp.StatusCode != expected {
		return "down", latencyMs, fmt.Sprintf("status %d (expected %d)", resp.StatusCode, expected)
	}
	return "up", latencyMs, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

func checkTCP(ctx context.Context, cfg healthCheckConfig, start time.Time) (string, int, string) {
	if cfg.TCPHost == "" || cfg.TCPPort == 0 {
		return "down", 0, "tcp_host or tcp_port not configured"
	}

	addr := fmt.Sprintf("%s:%d", cfg.TCPHost, cfg.TCPPort)
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return "down", latencyMs, fmt.Sprintf("tcp connect %s: %v", addr, err)
	}
	conn.Close()
	return "up", latencyMs, fmt.Sprintf("TCP %s reachable", addr)
}

// dockerSocketClient is a package-level, reusable HTTP client that talks to the
// Docker Unix socket. Creating a new http.Transport on every checkProcess call
// leaks the underlying idle-connection goroutines (readLoop + writeLoop) because
// the abandoned transport keeps them alive indefinitely. One shared client with
// MaxIdleConns=1 and a finite IdleConnTimeout avoids any accumulation.
var dockerSocketClient = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", "/var/run/docker.sock")
		},
		MaxIdleConns:    1,
		IdleConnTimeout: 90 * time.Second,
	},
}

func checkProcess(ctx context.Context, cfg healthCheckConfig, start time.Time) (string, int, string) {
	name := cfg.ServiceName
	if strings.HasPrefix(name, "/") {
		name = strings.TrimPrefix(name, "/")
	}

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(reqCtx,
		http.MethodGet,
		"http://docker/containers/"+name+"/json",
		nil,
	)
	resp, err := dockerSocketClient.Do(req)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return "down", latencyMs, fmt.Sprintf("docker inspect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "down", latencyMs, "container not found"
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "down", latencyMs, fmt.Sprintf("docker API status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the container inspect response to check if it is actually running.
	var inspect struct {
		State struct {
			Running bool   `json:"Running"`
			Status  string `json:"Status"` // "running", "exited", "paused", etc.
		} `json:"State"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&inspect); err != nil {
		return "down", latencyMs, fmt.Sprintf("parse docker response: %v", err)
	}
	if !inspect.State.Running {
		return "down", latencyMs, fmt.Sprintf("container state: %s", inspect.State.Status)
	}
	return "up", latencyMs, "container running"
}
