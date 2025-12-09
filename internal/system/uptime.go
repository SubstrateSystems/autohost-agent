package system

import (
	"os"
	"strconv"
	"strings"
)

// GetUptimeSeconds returns the system uptime in seconds.
// On Linux, it reads from /proc/uptime.
func GetUptimeSeconds() (uint64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}

	// /proc/uptime format: "uptime_seconds idle_seconds"
	parts := strings.Fields(string(data))
	if len(parts) < 1 {
		return 0, nil
	}

	uptimeFloat, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}

	return uint64(uptimeFloat), nil
}
