package metrics

import (
	"autohost-agent/pkg/sysinfo"
)

// Collector collects system metrics.
type Collector struct{}

// NewCollector creates a new metrics collector.
func NewCollector() *Collector {
	return &Collector{}
}

// Collect gathers current system metrics from sysinfo package.
func (c *Collector) Collect() (*Metrics, error) {
	metrics := &Metrics{}

	// Get CPU metrics
	cpuUsage, err := sysinfo.GetCPUUsagePercent()
	if err != nil {
		return nil, err
	}
	metrics.CPUUsagePercent = cpuUsage

	// Get memory metrics
	memInfo, err := sysinfo.GetMemoryInfo()
	if err != nil {
		return nil, err
	}
	metrics.MemoryTotalBytes = memInfo.TotalBytes
	metrics.MemoryUsedBytes = memInfo.UsedBytes
	metrics.MemoryAvailableBytes = memInfo.AvailableBytes
	metrics.MemoryUsagePercent = memInfo.UsagePercent

	// Get disk metrics for root partition
	diskInfo, err := sysinfo.GetDiskInfo("/")
	if err != nil {
		return nil, err
	}
	metrics.DiskTotalBytes = diskInfo.TotalBytes
	metrics.DiskUsedBytes = diskInfo.UsedBytes
	metrics.DiskAvailableBytes = diskInfo.AvailableBytes
	metrics.DiskUsagePercent = diskInfo.UsagePercent

	return metrics, nil
}
