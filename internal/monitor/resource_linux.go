//go:build linux

package monitor

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

type linuxResourceCollector struct{}

func newResourcePlatformCollector() resourcePlatformCollector {
	return linuxResourceCollector{}
}

func (linuxResourceCollector) CollectCPU(prev cpuTimes, hasPrev bool) (resourceMetric, cpuTimes, bool, error) {
	current, err := readCPUTimes()
	if err != nil {
		return resourceMetric{}, current, false, err
	}
	if !hasPrev {
		return resourceMetric{}, current, false, nil
	}
	usage, ok := cpuUsagePercent(prev, current)
	if !ok {
		return resourceMetric{}, current, false, nil
	}
	return resourceMetric{
		key:          resourceKey("cpu", ""),
		displayName:  "CPU",
		usagePercent: usage,
	}, current, true, nil
}

func (linuxResourceCollector) CollectMemory() (resourceMetric, error) {
	values, err := readMemInfo()
	if err != nil {
		return resourceMetric{}, err
	}
	totalKB := values["MemTotal"]
	availableKB := values["MemAvailable"]
	if totalKB == 0 {
		return resourceMetric{}, fmt.Errorf("MemTotal is empty")
	}
	if availableKB == 0 {
		availableKB = values["MemFree"] + values["Buffers"] + values["Cached"]
	}

	total := totalKB * 1024
	available := availableKB * 1024
	if available > total {
		available = total
	}
	used := total - available

	return resourceMetric{
		key:            resourceKey("memory", ""),
		displayName:    "内存",
		usagePercent:   percent(used, total),
		usedBytes:      used,
		availableBytes: available,
		totalBytes:     total,
	}, nil
}

func (linuxResourceCollector) CollectDisks(paths []string) ([]resourceMetric, error) {
	metrics := make([]resourceMetric, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		var stat syscall.Statfs_t
		if err := syscall.Statfs(path, &stat); err != nil {
			return metrics, fmt.Errorf("statfs %s: %w", path, err)
		}

		blockSize := uint64(stat.Bsize)
		total := stat.Blocks * blockSize
		available := stat.Bavail * blockSize
		if available > total {
			available = total
		}
		used := total - available

		metrics = append(metrics, resourceMetric{
			key:            resourceKey("disk", path),
			displayName:    "磁盘",
			path:           path,
			usagePercent:   percent(used, total),
			usedBytes:      used,
			availableBytes: available,
			totalBytes:     total,
		})
	}
	return metrics, nil
}

func readCPUTimes() (cpuTimes, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuTimes{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return cpuTimes{}, err
		}
		return cpuTimes{}, fmt.Errorf("/proc/stat is empty")
	}

	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuTimes{}, fmt.Errorf("unexpected /proc/stat cpu line")
	}

	values := make([]uint64, 0, len(fields)-1)
	for _, raw := range fields[1:] {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return cpuTimes{}, fmt.Errorf("parse cpu time %q: %w", raw, err)
		}
		values = append(values, v)
	}

	var total uint64
	for _, v := range values {
		total += v
	}
	idle := values[3]
	if len(values) > 4 {
		idle += values[4]
	}

	return cpuTimes{idle: idle, total: total}, nil
}

func readMemInfo() (map[string]uint64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	values := map[string]uint64{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse meminfo %s: %w", key, err)
		}
		values[key] = v
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}
