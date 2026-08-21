package monitor

import (
	"context"
	"fmt"
	"log"
	"time"
)

type AlertKind string

const (
	AlertKindLog      AlertKind = "log"
	AlertKindResource AlertKind = "resource"
)

type ResourceAlertStatus string

const (
	ResourceStatusAlert    ResourceAlertStatus = "告警"
	ResourceStatusOngoing  ResourceAlertStatus = "持续异常"
	ResourceStatusRecovery ResourceAlertStatus = "恢复"
)

type ResourceAlert struct {
	ServerName       string
	Metric           string
	Status           ResourceAlertStatus
	UsagePercent     float64
	ThresholdPercent float64
	RecoverPercent   float64
	Path             string
	UsedBytes        uint64
	AvailableBytes   uint64
	TotalBytes       uint64
	ObservedAt       time.Time
}

type resourceMetric struct {
	key            string
	displayName    string
	path           string
	usagePercent   float64
	usedBytes      uint64
	availableBytes uint64
	totalBytes     uint64
}

type resourceState struct {
	active   bool
	lastSent time.Time
}

type cpuTimes struct {
	idle  uint64
	total uint64
}

type resourceMonitor struct {
	cfg      *Config
	alerts   chan<- Alert
	logger   *log.Logger
	states   map[string]resourceState
	prevCPU  cpuTimes
	hasCPU   bool
	dropped  int
	platform resourcePlatformCollector
}

type resourcePlatformCollector interface {
	CollectCPU(prev cpuTimes, hasPrev bool) (resourceMetric, cpuTimes, bool, error)
	CollectMemory() (resourceMetric, error)
	CollectDisks(paths []string) ([]resourceMetric, error)
}

func RunResourceMonitor(ctx context.Context, cfg *Config, alerts chan<- Alert, logger *log.Logger, once bool) {
	if !cfg.ResourceMonitor.Enabled {
		return
	}

	monitor := &resourceMonitor{
		cfg:      cfg,
		alerts:   alerts,
		logger:   logger,
		states:   map[string]resourceState{},
		platform: newResourcePlatformCollector(),
	}

	interval := cfg.ResourceMonitorInterval()
	if interval <= 0 {
		interval = time.Minute
	}

	monitor.poll()
	if once {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			monitor.poll()
		}
	}
}

func (m *resourceMonitor) poll() {
	now := time.Now()
	metrics := make([]resourceMetric, 0, 2+len(m.cfg.ResourceMonitor.DiskPaths))

	if metric, nextCPU, ok, err := m.platform.CollectCPU(m.prevCPU, m.hasCPU); err != nil {
		m.logger.Printf("resource monitor collect cpu failed: %v", err)
	} else {
		m.prevCPU = nextCPU
		m.hasCPU = true
		if ok {
			metrics = append(metrics, metric)
		}
	}

	if metric, err := m.platform.CollectMemory(); err != nil {
		m.logger.Printf("resource monitor collect memory failed: %v", err)
	} else {
		metrics = append(metrics, metric)
	}

	diskMetrics, err := m.platform.CollectDisks(m.cfg.ResourceMonitor.DiskPaths)
	if err != nil {
		m.logger.Printf("resource monitor collect disk failed: %v", err)
	}
	metrics = append(metrics, diskMetrics...)

	for _, metric := range metrics {
		m.evaluateMetric(now, metric)
	}
}

func (m *resourceMonitor) evaluateMetric(now time.Time, metric resourceMetric) {
	threshold := m.cfg.ResourceMonitor.ThresholdPercent
	recover := m.cfg.ResourceMonitor.RecoverPercent
	cooldown := m.cfg.ResourceMonitorCooldown()
	if cooldown <= 0 {
		cooldown = 10 * time.Minute
	}

	state := m.states[metric.key]
	if metric.usagePercent >= threshold {
		status := ResourceStatusAlert
		shouldSend := !state.active
		if state.active && now.Sub(state.lastSent) >= cooldown {
			status = ResourceStatusOngoing
			shouldSend = true
		}
		state.active = true
		if shouldSend {
			state.lastSent = now
			m.enqueueResource(metric, status, now)
		}
		m.states[metric.key] = state
		return
	}

	if state.active && metric.usagePercent <= recover {
		state.active = false
		if m.cfg.ShouldNotifyResourceRecovery() {
			state.lastSent = now
			m.enqueueResource(metric, ResourceStatusRecovery, now)
		}
		m.states[metric.key] = state
		return
	}

	m.states[metric.key] = state
}

func (m *resourceMonitor) enqueueResource(metric resourceMetric, status ResourceAlertStatus, observedAt time.Time) {
	alert := Alert{
		Kind: AlertKindResource,
		Resource: &ResourceAlert{
			ServerName:       m.cfg.ResourceServerName(),
			Metric:           metric.displayName,
			Status:           status,
			UsagePercent:     metric.usagePercent,
			ThresholdPercent: m.cfg.ResourceMonitor.ThresholdPercent,
			RecoverPercent:   m.cfg.ResourceMonitor.RecoverPercent,
			Path:             metric.path,
			UsedBytes:        metric.usedBytes,
			AvailableBytes:   metric.availableBytes,
			TotalBytes:       metric.totalBytes,
			ObservedAt:       observedAt,
		},
	}

	select {
	case m.alerts <- alert:
	default:
		m.dropped++
		if m.dropped == 1 || m.dropped%100 == 0 {
			m.logger.Printf("resource alert queue full, dropped %d alert(s)", m.dropped)
		}
	}
}

func cpuUsagePercent(prev, current cpuTimes) (float64, bool) {
	totalDelta := current.total - prev.total
	idleDelta := current.idle - prev.idle
	if totalDelta == 0 || idleDelta > totalDelta {
		return 0, false
	}
	return float64(totalDelta-idleDelta) * 100 / float64(totalDelta), true
}

func percent(used, total uint64) float64 {
	if total == 0 || used > total {
		return 0
	}
	return float64(used) * 100 / float64(total)
}

func resourceKey(metric, path string) string {
	if path == "" {
		return metric
	}
	return fmt.Sprintf("%s:%s", metric, path)
}
