//go:build !linux

package monitor

import (
	"fmt"
	"runtime"
)

type unsupportedResourceCollector struct{}

func newResourcePlatformCollector() resourcePlatformCollector {
	return unsupportedResourceCollector{}
}

func (unsupportedResourceCollector) CollectCPU(prev cpuTimes, hasPrev bool) (resourceMetric, cpuTimes, bool, error) {
	return resourceMetric{}, cpuTimes{}, false, fmt.Errorf("resource monitor is only supported on linux, current platform: %s", runtime.GOOS)
}

func (unsupportedResourceCollector) CollectMemory() (resourceMetric, error) {
	return resourceMetric{}, fmt.Errorf("resource monitor is only supported on linux, current platform: %s", runtime.GOOS)
}

func (unsupportedResourceCollector) CollectDisks(paths []string) ([]resourceMetric, error) {
	return nil, fmt.Errorf("resource monitor is only supported on linux, current platform: %s", runtime.GOOS)
}
