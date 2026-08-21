package monitor

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type Matcher struct {
	include      []*regexp.Regexp
	exclude      []*regexp.Regexp
	contextAfter int
}

func NewMatcher(cfg MatchConfig) (*Matcher, error) {
	include, err := compileRegexes(cfg.IncludeRegex)
	if err != nil {
		return nil, fmt.Errorf("include_regex: %w", err)
	}
	exclude, err := compileRegexes(cfg.ExcludeRegex)
	if err != nil {
		return nil, fmt.Errorf("exclude_regex: %w", err)
	}
	return &Matcher{
		include:      include,
		exclude:      exclude,
		contextAfter: cfg.ContextLinesAfter,
	}, nil
}

func compileRegexes(patterns []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", pattern, err)
		}
		out = append(out, re)
	}
	return out, nil
}

func (m *Matcher) ContextLinesAfter() int {
	if m == nil {
		return 0
	}
	return m.contextAfter
}

func (m *Matcher) Match(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	for _, re := range m.exclude {
		if re.MatchString(line) {
			return false
		}
	}
	if level, ok := jsonLogLevel(line); ok {
		return isSendableJSONLevel(level)
	}
	for _, re := range m.include {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

func jsonLogLevel(line string) (string, bool) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		return "", false
	}
	level, _ := obj["level"].(string)
	if strings.TrimSpace(level) == "" {
		return "", false
	}
	return level, true
}

func isSendableJSONLevel(level string) bool {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "ERROR", "FATAL", "PANIC":
		return true
	default:
		return false
	}
}
