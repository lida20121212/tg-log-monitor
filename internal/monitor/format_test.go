package monitor

import (
	"strings"
	"testing"
	"time"
)

func TestBuildMessagesSplitsLongMessages(t *testing.T) {
	alert := Alert{
		SourceName: "server-1",
		Path:       "/logs/2026-08-11/app.log",
		Offset:     10,
		ObservedAt: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
		Lines:      []string{strings.Repeat("x", 200)},
	}
	messages := BuildMessages([]Alert{alert}, 80)
	if len(messages) < 2 {
		t.Fatalf("expected split messages, got %d", len(messages))
	}
	for _, msg := range messages {
		if len([]rune(msg)) > 80 {
			t.Fatalf("message too long: %d", len([]rune(msg)))
		}
	}
}

func TestFormatAlertIncludesZapStacktrace(t *testing.T) {
	alert := Alert{
		SourceName: "server-1",
		Path:       "/logs/2026-08-11/app.log",
		Offset:     10,
		ObservedAt: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
		Lines: []string{
			`{"level":"ERROR","time":"2026-08-11T12:00:00+0800","caller":"main.go:9","msg":"failed","stacktrace":"main.main\n\t/app/main.go:9"}`,
		},
	}
	got := FormatAlert(alert)
	if !strings.Contains(got, "log: app.log") {
		t.Fatalf("expected log file name in formatted alert, got:\n%s", got)
	}
	if !strings.Contains(got, "stacktrace: main.main") {
		t.Fatalf("expected stacktrace in formatted alert, got:\n%s", got)
	}
}

func TestFormatAlertStripsANSIColorCodes(t *testing.T) {
	alert := Alert{
		SourceName: "server-1",
		Path:       "/logs/2026-08-21/app.log",
		Offset:     10,
		ObservedAt: time.Date(2026, 8, 21, 14, 9, 34, 0, time.UTC),
		Lines: []string{
			"2026-08-21T14:09:34.643+0530\t\x1b[31mERROR\x1b[0m\tpaymentchannel/http_request.go:162\tpayment channel http request failed",
		},
	}
	got := FormatAlert(alert)
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("expected ANSI color codes stripped, got:\n%s", got)
	}
	if !strings.Contains(got, "\tERROR\t") {
		t.Fatalf("expected readable ERROR level, got:\n%s", got)
	}
}
