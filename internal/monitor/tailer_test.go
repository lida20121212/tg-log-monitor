package monitor

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTailerStartsAtEndThenAlertsOnAppend(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "app.log")
	if err := os.WriteFile(logPath, []byte(`{"level":"ERROR","msg":"old"}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	startAtEnd := true
	cfg := &Config{
		Sources: []SourceConfig{{
			Name:       "server-1",
			DirectFile: logPath,
		}},
		StateFile:  filepath.Join(dir, "state.json"),
		StartAtEnd: &startAtEnd,
		DryRun:     true,
	}
	cfg.applyDefaults()

	store, err := LoadState(cfg.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	matcher, err := NewMatcher(cfg.Match)
	if err != nil {
		t.Fatal(err)
	}

	alerts := make(chan Alert, 10)
	tailer := &sourceTailer{
		cfg:      cfg,
		source:   cfg.Sources[0],
		store:    store,
		matcher:  matcher,
		alerts:   alerts,
		logger:   log.New(os.Stdout, "", 0),
		location: time.Local,
	}

	tailer.poll(context.Background())
	select {
	case alert := <-alerts:
		t.Fatalf("expected initial existing error to be skipped, got %#v", alert)
	default:
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"level":"ERROR","msg":"new"}` + "\n"); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	tailer.poll(context.Background())
	select {
	case alert := <-alerts:
		got := strings.Join(alert.Lines, "\n")
		if strings.Contains(got, "old") || !strings.Contains(got, "new") {
			t.Fatalf("expected only new error, got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected appended error alert")
	}
}

func TestTailerDropsWhenAlertQueueFull(t *testing.T) {
	matcher, err := NewMatcher(MatchConfig{})
	if err != nil {
		t.Fatal(err)
	}

	alerts := make(chan Alert, 1)
	alerts <- Alert{SourceName: "prefill"}

	tailer := &sourceTailer{
		source:      SourceConfig{Name: "server-1"},
		matcher:     matcher,
		alerts:      alerts,
		logger:      log.New(io.Discard, "", 0),
		currentPath: "/tmp/app.log",
	}

	done := make(chan struct{})
	go func() {
		tailer.processLine(`{"level":"ERROR","msg":"boom"}`, 123)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("processLine blocked on full alert queue")
	}

	if got := len(alerts); got != 1 {
		t.Fatalf("expected queue length 1, got %d", got)
	}
	if tailer.droppedAlerts != 1 {
		t.Fatalf("expected 1 dropped alert, got %d", tailer.droppedAlerts)
	}
}
