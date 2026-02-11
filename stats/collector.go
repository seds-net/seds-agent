package stats

import (
	"encoding/json"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// SystemStats represents system statistics
type SystemStats struct {
	Hostname  string       `json:"hostname"`
	Platform  string       `json:"platform"`
	CPU       CPUStats     `json:"cpu"`
	Memory    MemoryStats  `json:"memory"`
	Disk      DiskStats    `json:"disk"`
	Network   NetworkStats `json:"network"`
	Uptime    uint64       `json:"uptime"`
	Timestamp int64        `json:"timestamp"`
}

// CPUStats represents CPU statistics
type CPUStats struct {
	Model       string    `json:"model"`
	Cores       int       `json:"cores"`
	UsagePercent float64  `json:"usage_percent"`
}

// MemoryStats represents memory statistics
type MemoryStats struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

// DiskStats represents disk statistics
type DiskStats struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

// NetworkStats represents network statistics
type NetworkStats struct {
	BytesSent uint64 `json:"bytes_sent"`
	BytesRecv uint64 `json:"bytes_recv"`
}

// Collector collects system statistics
type Collector struct {
	lastNetStats *NetworkStats
	lastNetTime  time.Time
}

// NewCollector creates a new stats collector
func NewCollector() *Collector {
	return &Collector{}
}

// Collect collects current system statistics
func (c *Collector) Collect() (json.RawMessage, error) {
	stats := &SystemStats{
		Timestamp: time.Now().Unix(),
	}

	// Host info
	hostInfo, err := host.Info()
	if err == nil {
		stats.Hostname = hostInfo.Hostname
		stats.Platform = hostInfo.Platform + " " + hostInfo.PlatformVersion
		stats.Uptime = hostInfo.Uptime
	}

	// CPU info
	cpuInfo, err := cpu.Info()
	if err == nil && len(cpuInfo) > 0 {
		stats.CPU.Model = cpuInfo[0].ModelName
		stats.CPU.Cores = runtime.NumCPU()
	}

	// CPU usage
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err == nil && len(cpuPercent) > 0 {
		stats.CPU.UsagePercent = cpuPercent[0]
	}

	// Memory info
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		stats.Memory.Total = memInfo.Total
		stats.Memory.Used = memInfo.Used
		stats.Memory.Free = memInfo.Free
		stats.Memory.UsedPercent = memInfo.UsedPercent
	}

	// Disk info (root partition)
	diskInfo, err := disk.Usage("/")
	if err == nil {
		stats.Disk.Total = diskInfo.Total
		stats.Disk.Used = diskInfo.Used
		stats.Disk.Free = diskInfo.Free
		stats.Disk.UsedPercent = diskInfo.UsedPercent
	}

	// Network info
	netIO, err := net.IOCounters(false)
	if err == nil && len(netIO) > 0 {
		stats.Network.BytesSent = netIO[0].BytesSent
		stats.Network.BytesRecv = netIO[0].BytesRecv
	}

	// Marshal to JSON
	data, err := json.Marshal(stats)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(data), nil
}
