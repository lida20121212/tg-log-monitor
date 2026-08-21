package monitor

import (
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"
)

func TestCPUUsagePercentUsesWholeMachineCapacity(t *testing.T) {
	prev := cpuTimes{idle: 100, total: 1000}
	current := cpuTimes{idle: 120, total: 1100}

	got, ok := cpuUsagePercent(prev, current)
	if !ok {
		t.Fatal("expected valid cpu usage")
	}
	if got != 80 {
		t.Fatalf("cpu usage = %.1f, want 80.0", got)
	}
}

func TestResourceMonitorThresholdCooldownAndRecovery(t *testing.T) {
	recovery := true
	cfg := &Config{
		Sources: []SourceConfig{{Name: "server-1"}},
		ResourceMonitor: ResourceMonitorConfig{
			Enabled:          true,
			ThresholdPercent: 80,
			RecoverPercent:   75,
			Cooldown:         "10m",
			NotifyRecovery:   &recovery,
		},
		StateFile: filepath.Join(t.TempDir(), "state.json"),
		DryRun:    true,
	}
	cfg.applyDefaults()

	alerts := make(chan Alert, 10)
	monitor := &resourceMonitor{
		cfg:    cfg,
		alerts: alerts,
		logger: log.New(io.Discard, "", 0),
		states: map[string]resourceState{},
	}

	metric := resourceMetric{
		key:          resourceKey("memory", ""),
		displayName:  "内存",
		usagePercent: 85,
	}
	start := time.Date(2026, 8, 21, 18, 0, 0, 0, time.UTC)

	monitor.evaluateMetric(start, metric)
	assertResourceAlert(t, alerts, ResourceStatusAlert)

	monitor.evaluateMetric(start.Add(time.Minute), metric)
	assertNoResourceAlert(t, alerts)

	monitor.evaluateMetric(start.Add(11*time.Minute), metric)
	assertResourceAlert(t, alerts, ResourceStatusOngoing)

	metric.usagePercent = 74
	monitor.evaluateMetric(start.Add(12*time.Minute), metric)
	assertResourceAlert(t, alerts, ResourceStatusRecovery)
}

func assertResourceAlert(t *testing.T, alerts <-chan Alert, status ResourceAlertStatus) {
	t.Helper()
	select {
	case alert := <-alerts:
		if alert.Kind != AlertKindResource || alert.Resource == nil {
			t.Fatalf("expected resource alert, got %#v", alert)
		}
		if alert.Resource.Status != status {
			t.Fatalf("resource status = %q, want %q", alert.Resource.Status, status)
		}
	default:
		t.Fatalf("expected resource alert status %q", status)
	}
}

func assertNoResourceAlert(t *testing.T, alerts <-chan Alert) {
	t.Helper()
	select {
	case alert := <-alerts:
		t.Fatalf("expected no resource alert, got %#v", alert)
	default:
	}
}
