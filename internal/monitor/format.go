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
		if strings.TrimSpace(text) == "" {
			continue
		}
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
	if alert.Kind == AlertKindResource || alert.Resource != nil {
		return FormatResourceAlert(alert.Resource)
	}

	var b strings.Builder
	b.WriteString("[错误日志告警]\n")
	b.WriteString("服务器: ")
	b.WriteString(alert.SourceName)
	b.WriteString("\n")
	b.WriteString("日志文件: ")
	b.WriteString(filepath.Base(alert.Path))
	b.WriteString("\n")
	b.WriteString("完整路径: ")
	b.WriteString(alert.Path)
	b.WriteString("\n")
	b.WriteString("检测时间: ")
	b.WriteString(alert.ObservedAt.Format("2006-01-02 15:04:05"))
	b.WriteString("\n")
	b.WriteString("读取位置: ")
	b.WriteString(fmt.Sprintf("%d", alert.Offset))
	b.WriteString("\n---\n")
	b.WriteString(formatLogLines(alert.Lines))
	return b.String()
}

func FormatResourceAlert(alert *ResourceAlert) string {
	if alert == nil {
		return ""
	}

	title := "[服务器资源告警]"
	if alert.Status == ResourceStatusRecovery {
		title = "[服务器资源恢复]"
	}

	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n服务器: ")
	b.WriteString(alert.ServerName)
	b.WriteString("\n状态: ")
	b.WriteString(string(alert.Status))
	b.WriteString("\n类型: ")
	b.WriteString(alert.Metric)
	if strings.TrimSpace(alert.Path) != "" {
		b.WriteString("\n路径: ")
		b.WriteString(alert.Path)
	}
	b.WriteString("\n使用率: ")
	b.WriteString(formatPercent(alert.UsagePercent))
	b.WriteString("\n阈值: ")
	b.WriteString(formatPercent(alert.ThresholdPercent))
	b.WriteString("\n恢复阈值: ")
	b.WriteString(formatPercent(alert.RecoverPercent))
	if alert.TotalBytes > 0 {
		b.WriteString("\n已用: ")
		b.WriteString(formatBytes(alert.UsedBytes))
		b.WriteString("\n可用: ")
		b.WriteString(formatBytes(alert.AvailableBytes))
		b.WriteString("\n总量: ")
		b.WriteString(formatBytes(alert.TotalBytes))
	}
	b.WriteString("\n检测时间: ")
	b.WriteString(alert.ObservedAt.Format("2006-01-02 15:04:05"))
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
			return summary + "\n上下文:\n" + strings.Join(cleaned[1:], "\n")
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
		b.WriteString(logFieldLabel(key))
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
			b.WriteString("其他字段: ")
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

func logFieldLabel(key string) string {
	switch key {
	case "level":
		return "级别"
	case "time":
		return "日志时间"
	case "caller":
		return "调用位置"
	case "msg":
		return "消息"
	case "error":
		return "错误"
	case "stacktrace", "stack":
		return "堆栈"
	default:
		return key
	}
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

func formatPercent(v float64) string {
	return fmt.Sprintf("%.1f%%", v)
}

func formatBytes(v uint64) string {
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%dB", v)
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	value := float64(v)
	for _, suffix := range units {
		value = value / unit
		if value < unit {
			return fmt.Sprintf("%.1f%s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1fEB", value/unit)
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
