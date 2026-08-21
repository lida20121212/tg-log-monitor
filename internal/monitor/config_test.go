package monitor

import (
	"os"
	"path/filepath"
	"testing"
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
