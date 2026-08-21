package monitor

import (
	"encoding/json"
	"fmt"
	"net/url"
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
	if summary := summarizeRequestLogLine(cleaned[0]); summary != "" {
		if len(lines) > 1 {
			return summary + "\n上下文:\n" + strings.Join(cleaned[1:], "\n")
		}
		return summary
	}
	if summary := summarizeJSONLog(cleaned[0]); summary != "" {
		if len(lines) > 1 {
			return summary + "\n上下文:\n" + strings.Join(cleaned[1:], "\n")
		}
		return summary
	}
	return strings.Join(cleaned, "\n")
}

type requestLogParts struct {
	Time   string
	Level  string
	Caller string
	Msg    string
	Fields map[string]any
}

func summarizeRequestLogLine(line string) string {
	if parts, ok := parseZapConsoleRequestLog(line); ok {
		return formatRequestLogSummary(parts)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &obj); err != nil {
		return ""
	}
	parts := requestLogParts{
		Time:   stringField(obj, "time", "ts", "timestamp"),
		Level:  stringField(obj, "level"),
		Caller: stringField(obj, "caller"),
		Msg:    stringField(obj, "msg", "message"),
		Fields: obj,
	}
	if !isRequestPayload(parts.Fields) {
		return ""
	}
	return formatRequestLogSummary(parts)
}

func parseZapConsoleRequestLog(line string) (requestLogParts, bool) {
	parts := strings.SplitN(strings.TrimSpace(line), "\t", 5)
	if len(parts) < 5 {
		return requestLogParts{}, false
	}

	var fields map[string]any
	rawFields := strings.TrimSpace(parts[4])
	if !strings.HasPrefix(rawFields, "{") {
		return requestLogParts{}, false
	}
	if err := json.Unmarshal([]byte(rawFields), &fields); err != nil {
		return requestLogParts{}, false
	}
	if !isRequestPayload(fields) {
		return requestLogParts{}, false
	}

	return requestLogParts{
		Time:   strings.TrimSpace(parts[0]),
		Level:  strings.TrimSpace(parts[1]),
		Caller: strings.TrimSpace(parts[2]),
		Msg:    strings.TrimSpace(parts[3]),
		Fields: fields,
	}, true
}

func isRequestPayload(fields map[string]any) bool {
	if len(fields) == 0 {
		return false
	}
	for _, key := range []string{
		"request_url",
		"request_params",
		"request_body",
		"response_params",
		"response_body",
		"response_status_code",
		"response_status",
		"request",
		"response",
		"path",
		"full_path",
		"request_path",
		"status_code",
	} {
		if _, ok := fields[key]; ok {
			return true
		}
	}
	return false
}

func formatRequestLogSummary(parts requestLogParts) string {
	var b strings.Builder
	appendSummaryField(&b, "级别", parts.Level)
	appendSummaryField(&b, "日志时间", parts.Time)
	appendSummaryField(&b, "调用位置", parts.Caller)
	appendSummaryField(&b, "消息", parts.Msg)

	method := stringField(parts.Fields, "method")
	requestURL := redactURL(stringField(parts.Fields, "request_url", "url", "full_path", "path", "request_path", "uri"))
	if method != "" || requestURL != "" {
		appendSummaryField(&b, "请求", strings.TrimSpace(method+" "+requestURL))
	}

	status := compactStatus(parts.Fields)
	appendSummaryField(&b, "HTTP状态", status)

	if responseNil, ok := parts.Fields["response_nil"]; ok {
		appendSummaryField(&b, "响应为空", valueToString(responseNil))
	}

	appendSummaryField(&b, "错误", compactText(stringField(parts.Fields, "error"), 260))
	appendSummaryField(&b, "请求参数", summarizePayload(stringField(parts.Fields, "request_params", "request_body", "request"), 180))
	responseSummary := summarizePayload(stringField(parts.Fields, "response_params", "response_body"), 260)
	if responseSummary == "" {
		responseSummary = summarizePayload(stringField(parts.Fields, "response"), 260)
	}
	appendSummaryField(&b, "响应摘要", responseSummary)

	out := strings.TrimRight(b.String(), "\n")
	if out == "" {
		return ""
	}
	return out
}

func compactStatus(fields map[string]any) string {
	code := stringField(fields, "response_status_code", "status_code", "code")
	status := stringField(fields, "response_status", "status")
	if code == "" {
		return status
	}
	if status == "" || status == code {
		return code
	}
	return strings.TrimSpace(code + " " + status)
}

func summarizePayload(raw string, limit int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if summary := summarizeJSONPayload(raw, limit); summary != "" {
		return summary
	}
	if summary := summarizeFormPayload(raw, limit); summary != "" {
		return summary
	}
	return compactText(raw, limit)
}

func summarizeJSONPayload(raw string, limit int) string {
	var obj any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return ""
	}
	redacted := redactSensitiveValue(obj)
	data, err := json.Marshal(redacted)
	if err != nil {
		return ""
	}
	return compactText(string(data), limit)
}

func summarizeFormPayload(raw string, limit int) string {
	if !strings.Contains(raw, "=") {
		return ""
	}
	values, err := url.ParseQuery(raw)
	if err != nil || len(values) == 0 {
		return ""
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		val := ""
		if len(values[key]) > 0 {
			val = values[key][0]
		}
		if isSensitiveKey(key) {
			val = "[已隐藏]"
		}
		parts = append(parts, key+"="+val)
	}
	return compactText(strings.Join(parts, "&"), limit)
}

func redactSensitiveValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for key, val := range x {
			if isSensitiveKey(key) {
				out[key] = "[已隐藏]"
				continue
			}
			out[key] = redactSensitiveValue(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = redactSensitiveValue(val)
		}
		return out
	case string:
		return compactText(x, 90)
	default:
		return v
	}
}

func redactURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return compactText(raw, 260)
	}
	query := u.Query()
	changed := false
	for key := range query {
		if isSensitiveKey(key) {
			query.Set(key, "[已隐藏]")
			changed = true
		}
	}
	if changed {
		u.RawQuery = query.Encode()
	}
	return compactText(u.String(), 260)
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	return strings.Contains(key, "sign") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "token") ||
		strings.Contains(key, "password") ||
		key == "key" ||
		key == "apikey" ||
		key == "api_key" ||
		strings.HasSuffix(key, "_key")
}

func appendSummaryField(b *strings.Builder, label, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	b.WriteString(label)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteString("\n")
}

func compactText(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if maxRunes <= 0 || runeLen(s) <= maxRunes {
		return s
	}
	part, _ := splitRunes(s, maxRunes)
	return strings.TrimRight(part, " ") + "...(已截断)"
}

func stringField(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		val, ok := obj[key]
		if !ok {
			continue
		}
		s := valueToString(val)
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
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
