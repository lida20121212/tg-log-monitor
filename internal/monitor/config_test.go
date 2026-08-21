package monitor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigAllowsComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := `{
  // line comment
  "telegram": {
    "bot_token": "",
    "chat_id": 0,
    "parse_mode": ""
  },
  "sources": [
    {
      "name": "server//1",
      "direct_file": "C:\\logs\\app.log"
    }
  ],
  /*
    block comment
  */
  "match": {
    "include_regex": ["panic:"]
  },
  "dry_run": true
}`

	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Sources[0].Name; got != "server//1" {
		t.Fatalf("source name = %q, want server//1", got)
	}
	if !cfg.DryRun {
		t.Fatal("expected dry_run=true")
	}
}

func TestSourceCurrentPathsMatchesAllLogFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "2026-08-21")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"b.log", "a.log", "skip.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	source := SourceConfig{
		Name:        "server-1",
		LogRoot:     root,
		FilePattern: "*.log",
		DateLayout:  "2006-01-02",
	}
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)
	paths, err := source.CurrentPaths(now)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		filepath.Join(dir, "a.log"),
		filepath.Join(dir, "b.log"),
	}
	if len(paths) != len(want) {
		t.Fatalf("paths len = %d, want %d: %#v", len(paths), len(want), paths)
	}
	for i := range want {
		if paths[i] != filepath.Clean(want[i]) {
			t.Fatalf("paths[%d] = %q, want %q", i, paths[i], filepath.Clean(want[i]))
		}
	}
}

func TestSourceCurrentPathsKeepsLegacyFileName(t *testing.T) {
	source := SourceConfig{
		Name:       "server-1",
		LogRoot:    `C:\logs`,
		FileName:   "app.log",
		DateLayout: "2006-01-02",
	}
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)
	paths, err := source.CurrentPaths(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("paths len = %d, want 1", len(paths))
	}
	if got := filepath.Base(paths[0]); got != "app.log" {
		t.Fatalf("base path = %q, want app.log", got)
	}
}

func TestExampleConfigLoads(t *testing.T) {
	t.Setenv("DRY_RUN", "1")
	cfg, err := LoadConfig(filepath.Join("..", "..", "config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) == 0 {
		t.Fatal("expected at least one source")
	}
	for i, source := range cfg.Sources {
		if got := source.DateLayout; got != "2006-01-02" {
			t.Fatalf("sources[%d].date_layout = %q, want 2006-01-02", i, got)
		}
	}
}
