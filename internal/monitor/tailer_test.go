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
	watcher := &sourceWatcher{
		cfg:      cfg,
		source:   cfg.Sources[0],
		store:    store,
		matcher:  matcher,
		alerts:   alerts,
		logger:   log.New(os.Stdout, "", 0),
		location: time.Local,
		tailers:  map[string]*sourceTailer{},
	}

	watcher.poll(context.Background())
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

	watcher.poll(context.Background())
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

func TestTailerWatchesAllLogFilesInDateDir(t *testing.T) {
	root := t.TempDir()
	dateDir := filepath.Join(root, time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(dateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	aLog := filepath.Join(dateDir, "a.log")
	bLog := filepath.Join(dateDir, "b.log")
	ignored := filepath.Join(dateDir, "ignore.txt")
	for path, body := range map[string]string{
		aLog:    `{"level":"ERROR","msg":"old-a"}` + "\n",
		bLog:    `{"level":"ERROR","msg":"old-b"}` + "\n",
		ignored: `{"level":"ERROR","msg":"ignored"}` + "\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	startAtEnd := true
	cfg := &Config{
		Sources: []SourceConfig{{
			Name:        "server-1",
			LogRoot:     root,
			FilePattern: "*.log",
			DateLayout:  "2006-01-02",
		}},
		StateFile:  filepath.Join(root, "state.json"),
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
	watcher := &sourceWatcher{
		cfg:      cfg,
		source:   cfg.Sources[0],
		store:    store,
		matcher:  matcher,
		alerts:   alerts,
		logger:   log.New(io.Discard, "", 0),
		location: time.Local,
		tailers:  map[string]*sourceTailer{},
	}

	watcher.poll(context.Background())
	select {
	case alert := <-alerts:
		t.Fatalf("expected initial existing errors to be skipped, got %#v", alert)
	default:
	}

	appendLine := func(path, line string) {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(line + "\n"); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	appendLine(aLog, `{"level":"ERROR","msg":"new-a"}`)
	appendLine(bLog, `{"level":"ERROR","msg":"new-b"}`)
	cLog := filepath.Join(dateDir, "c.log")
	if err := os.WriteFile(cLog, []byte(`{"level":"ERROR","msg":"new-c"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	watcher.poll(context.Background())
	got := map[string]string{}
	deadline := time.After(time.Second)
	for len(got) < 3 {
		select {
		case alert := <-alerts:
			got[filepath.Base(alert.Path)] = strings.Join(alert.Lines, "\n")
		case <-deadline:
			t.Fatalf("expected alerts for a.log, b.log, and c.log; got %#v", got)
		}
	}

	for name, msg := range map[string]string{
		"a.log": "new-a",
		"b.log": "new-b",
		"c.log": "new-c",
	} {
		if !strings.Contains(got[name], msg) {
			t.Fatalf("%s alert = %q, want %s", name, got[name], msg)
		}
	}
	for _, line := range got {
		if strings.Contains(line, "old-") || strings.Contains(line, "ignored") {
			t.Fatalf("unexpected historical or ignored log in alerts: %#v", got)
		}
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
