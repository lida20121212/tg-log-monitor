package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Telegram        TelegramConfig `json:"telegram"`
	Sources         []SourceConfig `json:"sources"`
	Match           MatchConfig    `json:"match"`
	PollInterval    string         `json:"poll_interval"`
	SendInterval    string         `json:"send_interval"`
	MaxBatchLines   int            `json:"max_batch_lines"`
	StateFile       string         `json:"state_file"`
	StartAtEnd      *bool          `json:"start_at_end"`
	MessageMaxChars int            `json:"message_max_chars"`
	HTTPTimeout     string         `json:"http_timeout"`
	DryRun          bool           `json:"dry_run"`

	configDir string
}

type TelegramConfig struct {
	BotToken  string     `json:"bot_token"`
	ChatID    Int64Value `json:"chat_id"`
	ParseMode string     `json:"parse_mode"`
}

type SourceConfig struct {
	Name       string `json:"name"`
	LogRoot    string `json:"log_root"`
	FileName   string `json:"file_name"`
	DateLayout string `json:"date_layout"`
	Timezone   string `json:"timezone"`
	DirectFile string `json:"direct_file"`
}

type MatchConfig struct {
	IncludeRegex      []string `json:"include_regex"`
	ExcludeRegex      []string `json:"exclude_regex"`
	ContextLinesAfter int      `json:"context_lines_after"`
}

const (
	excludeResponseCode40102      = `"response"\s*:\s*\{[^}\n]*"code"\s*:\s*40102\b`
	excludeVerificationFailedCode = `"error"\s*:\s*"Verification failed"\s*,\s*"response"\s*:\s*\{[^}\n]*"code"\s*:\s*1\b[^}\n]*"msg"\s*:\s*"Verification failed"`
)

type Int64Value int64

func (v *Int64Value) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		*v = 0
		return nil
	}
	if strings.HasPrefix(raw, "\"") {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		raw = strings.TrimSpace(s)
		if raw == "" {
			*v = 0
			return nil
		}
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid int64 value %q: %w", raw, err)
	}
	*v = Int64Value(n)
	return nil
}

func (v Int64Value) Int64() int64 {
	return int64(v)
}

func LoadConfig(path string) (*Config, error) {
	if strings.TrimSpace(path) == "" {
		path = "config.json"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	abs, err := filepath.Abs(path)
	if err == nil {
		cfg.configDir = filepath.Dir(abs)
	}
	cfg.applyDefaults()
	cfg.applyEnv()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.configDir == "" {
		c.configDir, _ = os.Getwd()
	}
	if c.PollInterval == "" {
		c.PollInterval = "2s"
	}
	if c.SendInterval == "" {
		c.SendInterval = "5s"
	}
	if c.HTTPTimeout == "" {
		c.HTTPTimeout = "5s"
	}
	if c.MaxBatchLines <= 0 {
		c.MaxBatchLines = 20
	}
	if c.MessageMaxChars <= 0 {
		c.MessageMaxChars = 3500
	}
	if c.StateFile == "" {
		c.StateFile = "state.json"
	}
	if !filepath.IsAbs(c.StateFile) {
		c.StateFile = filepath.Join(c.configDir, c.StateFile)
	}
	if len(c.Match.IncludeRegex) == 0 {
		c.Match.IncludeRegex = []string{
			`"level"\s*:\s*"ERROR"`,
			`"level"\s*:\s*"FATAL"`,
			`"level"\s*:\s*"PANIC"`,
			`\bERROR\b`,
			`\bFATAL\b`,
			`\bPANIC\b`,
			`panic:`,
		}
	}
	c.Match.ExcludeRegex = appendRegexIfMissing(c.Match.ExcludeRegex, excludeResponseCode40102)
	c.Match.ExcludeRegex = appendRegexIfMissing(c.Match.ExcludeRegex, excludeVerificationFailedCode)
	if c.Match.ContextLinesAfter < 0 {
		c.Match.ContextLinesAfter = 0
	}

	for i := range c.Sources {
		if strings.TrimSpace(c.Sources[i].Name) == "" {
			c.Sources[i].Name = fmt.Sprintf("server-%d", i+1)
		}
		if c.Sources[i].FileName == "" {
			c.Sources[i].FileName = "app.log"
		}
		if c.Sources[i].DateLayout == "" {
			c.Sources[i].DateLayout = "2006-01-02"
		}
	}
}

func appendRegexIfMissing(regexes []string, pattern string) []string {
	for _, re := range regexes {
		if re == pattern {
			return regexes
		}
	}
	return append(regexes, pattern)
}

func (c *Config) applyEnv() {
	if v := strings.TrimSpace(os.Getenv("TG_BOT_TOKEN")); v != "" {
		c.Telegram.BotToken = v
	}
	if v := strings.TrimSpace(os.Getenv("TG_CHAT_ID")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			c.Telegram.ChatID = Int64Value(n)
		}
	}
	if v := strings.TrimSpace(os.Getenv("TG_PARSE_MODE")); v != "" {
		c.Telegram.ParseMode = v
	}
	if v := strings.TrimSpace(os.Getenv("STATE_FILE")); v != "" {
		c.StateFile = v
		if !filepath.IsAbs(c.StateFile) {
			c.StateFile = filepath.Join(c.configDir, c.StateFile)
		}
	}
	if v := strings.TrimSpace(os.Getenv("DRY_RUN")); v != "" {
		c.DryRun = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}

	envLogRoot := strings.TrimSpace(os.Getenv("LOG_ROOT"))
	envServerName := strings.TrimSpace(os.Getenv("SERVER_NAME"))
	if envLogRoot != "" {
		if len(c.Sources) == 0 {
			c.Sources = []SourceConfig{{
				Name:       "server-1",
				LogRoot:    envLogRoot,
				FileName:   "app.log",
				DateLayout: "2006-01-02",
			}}
		} else {
			c.Sources[0].LogRoot = envLogRoot
		}
	}
	if envServerName != "" {
		if len(c.Sources) == 0 {
			c.Sources = []SourceConfig{{
				Name:       envServerName,
				FileName:   "app.log",
				DateLayout: "2006-01-02",
			}}
		} else {
			c.Sources[0].Name = envServerName
		}
	}
}

func (c *Config) validate() error {
	if len(c.Sources) == 0 {
		return fmt.Errorf("at least one source is required")
	}
	if !c.DryRun {
		if strings.TrimSpace(c.Telegram.BotToken) == "" {
			return fmt.Errorf("telegram.bot_token is required unless dry_run=true")
		}
		if c.Telegram.ChatID.Int64() == 0 {
			return fmt.Errorf("telegram.chat_id is required unless dry_run=true")
		}
	}
	if _, err := time.ParseDuration(c.PollInterval); err != nil {
		return fmt.Errorf("invalid poll_interval %q: %w", c.PollInterval, err)
	}
	if _, err := time.ParseDuration(c.SendInterval); err != nil {
		return fmt.Errorf("invalid send_interval %q: %w", c.SendInterval, err)
	}
	if _, err := time.ParseDuration(c.HTTPTimeout); err != nil {
		return fmt.Errorf("invalid http_timeout %q: %w", c.HTTPTimeout, err)
	}

	seenNames := map[string]bool{}
	for i, src := range c.Sources {
		name := strings.TrimSpace(src.Name)
		if name == "" {
			return fmt.Errorf("sources[%d].name is required", i)
		}
		if seenNames[name] {
			return fmt.Errorf("duplicate source name %q", name)
		}
		seenNames[name] = true
		if strings.TrimSpace(src.DirectFile) == "" && strings.TrimSpace(src.LogRoot) == "" {
			return fmt.Errorf("sources[%d].log_root is required when direct_file is empty", i)
		}
		if strings.TrimSpace(src.Timezone) != "" {
			if _, err := time.LoadLocation(src.Timezone); err != nil {
				return fmt.Errorf("sources[%d].timezone %q: %w", i, src.Timezone, err)
			}
		}
	}
	return nil
}

func (c *Config) PollDuration() time.Duration {
	d, _ := time.ParseDuration(c.PollInterval)
	return d
}

func (c *Config) SendDuration() time.Duration {
	d, _ := time.ParseDuration(c.SendInterval)
	return d
}

func (c *Config) HTTPTimeoutDuration() time.Duration {
	d, _ := time.ParseDuration(c.HTTPTimeout)
	return d
}

func (c *Config) ShouldStartAtEnd() bool {
	if c.StartAtEnd == nil {
		return true
	}
	return *c.StartAtEnd
}

func (s SourceConfig) Location() (*time.Location, error) {
	if strings.TrimSpace(s.Timezone) == "" {
		return time.Local, nil
	}
	return time.LoadLocation(s.Timezone)
}

func (s SourceConfig) CurrentPath(now time.Time) string {
	if strings.TrimSpace(s.DirectFile) != "" {
		return filepath.Clean(s.DirectFile)
	}
	date := now.Format(s.DateLayout)
	return filepath.Join(s.LogRoot, date, s.FileName)
}
