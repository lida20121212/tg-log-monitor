package monitor

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func BuildMessages(alerts []Alert, maxChars int) []string {
	if maxChars <= 0 {
		maxChars = 3500
	}

	var messages []string
	var current strings.Builder
	for _, alert := range alerts {
		text := FormatAlert(alert)
		if current.Len() > 0 && runeLen(current.String())+runeLen(text)+2 > maxChars {
			messages = append(messages, current.String())
			current.Reset()
		}
		for runeLen(text) > maxChars {
			part, rest := splitRunes(text, maxChars)
			if current.Len() > 0 {
				messages = append(messages, current.String())
				current.Reset()
			}
			messages = append(messages, part)
			text = rest
		}
		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(text)
	}
	if current.Len() > 0 {
		messages = append(messages, current.String())
	}
	return messages
}

func FormatAlert(alert Alert) string {
	var b strings.Builder
	b.WriteString("[ERROR LOG]\n")
	b.WriteString("server: ")
	b.WriteString(alert.SourceName)
	b.WriteString("\n")
	b.WriteString("log: ")
	b.WriteString(filepath.Base(alert.Path))
	b.WriteString("\n")
	b.WriteString("file: ")
	b.WriteString(alert.Path)
	b.WriteString("\n")
	b.WriteString("detected_at: ")
	b.WriteString(alert.ObservedAt.Format("2006-01-02 15:04:05"))
	b.WriteString("\n")
	b.WriteString("offset: ")
	b.WriteString(fmt.Sprintf("%d", alert.Offset))
	b.WriteString("\n---\n")
	b.WriteString(formatLogLines(alert.Lines))
	return b.String()
}

func formatLogLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		cleaned = append(cleaned, stripANSI(line))
	}
	if summary := summarizeJSONLog(cleaned[0]); summary != "" {
		if len(lines) > 1 {
			return summary + "\ncontext:\n" + strings.Join(cleaned[1:], "\n")
		}
		return summary
	}
	return strings.Join(cleaned, "\n")
}

func summarizeJSONLog(line string) string {
	var obj map[string]any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		return ""
	}

	keys := []string{"level", "time", "caller", "msg", "error", "stacktrace", "stack"}
	used := map[string]bool{}
	var b strings.Builder
	for _, key := range keys {
		val, ok := obj[key]
		if !ok {
			continue
		}
		used[key] = true
		s := valueToString(val)
		if strings.TrimSpace(s) == "" {
			continue
		}
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(s)
		b.WriteString("\n")
	}

	var extraKeys []string
	for key := range obj {
		if !used[key] {
			extraKeys = append(extraKeys, key)
		}
	}
	sort.Strings(extraKeys)
	if len(extraKeys) > 0 {
		extra := map[string]any{}
		for _, key := range extraKeys {
			extra[key] = obj[key]
		}
		if data, err := json.Marshal(extra); err == nil {
			b.WriteString("fields: ")
			b.Write(data)
			b.WriteString("\n")
		}
	}

	out := strings.TrimRight(b.String(), "\n")
	if out == "" {
		return line
	}
	return out
}

func valueToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		data, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(x)
		}
		return string(data)
	}
}

func runeLen(s string) int {
	return len([]rune(s))
}

func splitRunes(s string, n int) (string, string) {
	runes := []rune(s)
	if n >= len(runes) {
		return s, ""
	}
	return string(runes[:n]), string(runes[n:])
}
