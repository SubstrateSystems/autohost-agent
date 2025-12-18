package metrics

// Metrics represents system metrics that will be sent to the API.
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
