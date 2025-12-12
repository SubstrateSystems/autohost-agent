package system

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
)

type Metrics struct {
	CPUUsagePercent      float64 `json:"cpu_usage_percent"`
	MemoryTotalBytes     uint64  `json:"memory_total_bytes"`
	MemoryUsedBytes      uint64  `json:"memory_used_bytes"`
	MemoryAvailableBytes uint64  `json:"memory_available_bytes"`
	MemoryUsagePercent   float64 `json:"memory_usage_percent"`
	DiskTotalBytes       uint64  `json:"disk_total_bytes"`
	DiskUsedBytes        uint64  `json:"disk_used_bytes"`
	DiskAvailableBytes   uint64  `json:"disk_available_bytes"`
	DiskUsagePercent     float64 `json:"disk_usage_percent"`
}

// Variables para calcular CPU como delta entre lecturas
var (
	prevCPUTotal uint64
	prevCPUIdle  uint64
	hasPrevCPU   bool
)

// GetMetrics returns current system metrics.
func GetMetrics() (*Metrics, error) {
	metrics := &Metrics{}

	// Get memory metrics
	if err := getMemoryMetrics(metrics); err != nil {
		return nil, err
	}

	// Get disk metrics for root partition
	if err := getDiskMetrics(metrics, "/"); err != nil {
		return nil, err
	}

	// Get CPU metrics
	if err := getCPUMetrics(metrics); err != nil {
		return nil, err
	}

	return metrics, nil
}

// getMemoryMetrics reads /proc/meminfo to get memory statistics.
func getMemoryMetrics(m *Metrics) error {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return err
	}
	defer file.Close()

	memInfo := make(map[string]uint64)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		key := strings.TrimSuffix(parts[0], ":")
		value, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			continue
		}

		// Convert from KB to bytes
		memInfo[key] = value * 1024
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	m.MemoryTotalBytes = memInfo["MemTotal"]
	m.MemoryAvailableBytes = memInfo["MemAvailable"]

	// Fallback por si MemAvailable no existe (sistemas muy viejos)
	if m.MemoryAvailableBytes == 0 {
		free := memInfo["MemFree"]
		buffers := memInfo["Buffers"]
		cached := memInfo["Cached"]
		m.MemoryAvailableBytes = free + buffers + cached
	}

	m.MemoryUsedBytes = m.MemoryTotalBytes - m.MemoryAvailableBytes

	if m.MemoryTotalBytes > 0 {
		m.MemoryUsagePercent = float64(m.MemoryUsedBytes) / float64(m.MemoryTotalBytes) * 100
	}

	return nil
}

// getDiskMetrics uses syscall.Statfs to get disk statistics.
func getDiskMetrics(m *Metrics, path string) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return err
	}

	blockSize := uint64(stat.Bsize)

	m.DiskTotalBytes = stat.Blocks * blockSize

	// Bavail = bloques disponibles para usuario no-root
	m.DiskAvailableBytes = stat.Bavail * blockSize

	// Usado desde la perspectiva del usuario: total - available
	m.DiskUsedBytes = m.DiskTotalBytes - m.DiskAvailableBytes

	if m.DiskTotalBytes > 0 {
		m.DiskUsagePercent = float64(m.DiskUsedBytes) / float64(m.DiskTotalBytes) * 100
	}

	return nil
}

// getCPUMetrics reads /proc/stat and calculates CPU usage as a delta
// between this reading and the previous one.
func getCPUMetrics(m *Metrics) error {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return scanner.Err()
	}

	line := scanner.Text()
	if !strings.HasPrefix(line, "cpu ") {
		// No pudimos leer la línea global de CPU
		return nil
	}

	fields := strings.Fields(line)
	if len(fields) < 8 {
		return nil
	}

	// Parse CPU times: user, nice, system, idle, iowait, irq, softirq, steal
	var times [8]uint64
	for i := 0; i < 8 && i+1 < len(fields); i++ {
		val, err := strconv.ParseUint(fields[i+1], 10, 64)
		if err != nil {
			// Si algo falla, simplemente dejamos ese valor en 0
			continue
		}
		times[i] = val
	}

	// idle + iowait
	idle := times[3] + times[4]

	// total = suma de todos los campos
	total := uint64(0)
	for _, t := range times {
		total += t
	}

	// Si ya tenemos una lectura previa, calculamos el uso como delta
	if hasPrevCPU {
		deltaTotal := total - prevCPUTotal
		deltaIdle := idle - prevCPUIdle

		if deltaTotal > 0 {
			m.CPUUsagePercent = float64(deltaTotal-deltaIdle) / float64(deltaTotal) * 100
		}
	}

	// Guardamos para la siguiente llamada
	prevCPUTotal = total
	prevCPUIdle = idle
	hasPrevCPU = true

	return nil
}
