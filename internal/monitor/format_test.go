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
	if !strings.Contains(got, "日志文件: app.log") {
		t.Fatalf("expected log file name in formatted alert, got:\n%s", got)
	}
	if !strings.Contains(got, "堆栈: main.main") {
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

func TestFormatAlertSummarizesPaymentRequestLog(t *testing.T) {
	alert := Alert{
		SourceName: "server-1",
		Path:       "/logs/2026-08-21/app.log",
		Offset:     10,
		ObservedAt: time.Date(2026, 8, 21, 14, 9, 34, 0, time.UTC),
		Lines: []string{
			"2026-08-21T14:09:34.643+0530\t\x1b[31mERROR\x1b[0m\tpaymentchannel/http_request.go:162\tpayment channel http request failed\t{\"method\":\"POST\",\"request_url\":\"https://pay.example.com/payout/balance?sign=URLSIGN\",\"request_params\":\"{\\\"merchantNo\\\":\\\"M1001\\\",\\\"sign\\\":\\\"REQSIGN\\\",\\\"secretKey\\\":\\\"SECRETKEY\\\",\\\"body\\\":\\\"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz\\\"}\",\"request_params_truncated\":false,\"response_status_code\":403,\"response_status\":\"403 Forbidden\",\"response_params\":\"{\\\"code\\\":403,\\\"message\\\":\\\"ip forbidden\\\",\\\"sign\\\":\\\"RESPSIGN\\\"}\",\"response_params_truncated\":false,\"error\":\"payment channel http status 403\"}",
		},
	}

	got := FormatAlert(alert)
	for _, want := range []string{
		"级别: ERROR",
		"调用位置: paymentchannel/http_request.go:162",
		"消息: payment channel http request failed",
		"请求: POST https://pay.example.com/payout/balance",
		"HTTP状态: 403 403 Forbidden",
		"错误: payment channel http status 403",
		"请求参数: ",
		"响应摘要: ",
		"merchantNo",
		"[已隐藏]",
		"(已截断)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in summarized alert, got:\n%s", want, got)
		}
	}
	for _, secret := range []string{"URLSIGN", "REQSIGN", "SECRETKEY", "RESPSIGN", "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"} {
		if strings.Contains(got, secret) {
			t.Fatalf("expected secret/long payload %q to be hidden or truncated, got:\n%s", secret, got)
		}
	}
}

func TestFormatAlertSummarizesFormRequestParams(t *testing.T) {
	alert := Alert{
		SourceName: "server-1",
		Path:       "/logs/2026-08-21/app.log",
		Offset:     10,
		ObservedAt: time.Date(2026, 8, 21, 14, 9, 34, 0, time.UTC),
		Lines: []string{
			"2026-08-21T14:09:34.870+0530\tERROR\tpaymentchannel/http_request.go:162\tpayment channel http request failed\t{\"method\":\"POST\",\"request_url\":\"https://api.hhpays.neterror/payout/balance\",\"request_params\":\"merchant=M1784018428980&sign=FORM_SIGN\",\"response_nil\":true,\"error\":\"Post \\\"https://api.hhpays.neterror/payout/balance\\\": EOF\"}",
		},
	}

	got := FormatAlert(alert)
	if !strings.Contains(got, "请求参数: merchant=M1784018428980&sign=[已隐藏]") {
		t.Fatalf("expected concise redacted form params, got:\n%s", got)
	}
	if strings.Contains(got, "FORM_SIGN") {
		t.Fatalf("expected form sign hidden, got:\n%s", got)
	}
}

func TestFormatAlertSummarizesGameRequestJSONLog(t *testing.T) {
	alert := Alert{
		SourceName: "game-1",
		Path:       "/logs/2026-08-21/game.log",
		Offset:     10,
		ObservedAt: time.Date(2026, 8, 21, 14, 9, 34, 0, time.UTC),
		Lines: []string{
			`{"level":"ERROR","time":"2026-08-21T14:09:34.643+0530","caller":"game/http.go:88","msg":"game request failed","method":"POST","path":"/api/game/bet","request":{"orderNo":"O1001","amount":100,"sign":"GAME_SIGN"},"response":{"code":500,"message":"game rejected"}}`,
		},
	}

	got := FormatAlert(alert)
	for _, want := range []string{
		"级别: ERROR",
		"调用位置: game/http.go:88",
		"消息: game request failed",
		"请求: POST /api/game/bet",
		"请求参数: ",
		"响应摘要: ",
		"orderNo",
		"game rejected",
		"[已隐藏]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in summarized game alert, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "GAME_SIGN") {
		t.Fatalf("expected game request sign hidden, got:\n%s", got)
	}
}

func TestFormatResourceAlertUsesChineseFields(t *testing.T) {
	alert := &ResourceAlert{
		ServerName:       "server-1",
		Metric:           "磁盘",
		Status:           ResourceStatusAlert,
		UsagePercent:     85.2,
		ThresholdPercent: 80,
		RecoverPercent:   75,
		Path:             "/www/wwwroot",
		UsedBytes:        170 * 1024 * 1024 * 1024,
		AvailableBytes:   30 * 1024 * 1024 * 1024,
		TotalBytes:       200 * 1024 * 1024 * 1024,
		ObservedAt:       time.Date(2026, 8, 21, 18, 30, 0, 0, time.UTC),
	}

	got := FormatResourceAlert(alert)
	for _, want := range []string{
		"[服务器资源告警]",
		"服务器: server-1",
		"状态: 告警",
		"类型: 磁盘",
		"路径: /www/wwwroot",
		"使用率: 85.2%",
		"阈值: 80.0%",
		"恢复阈值: 75.0%",
		"已用: 170.0GB",
		"可用: 30.0GB",
		"总量: 200.0GB",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in resource alert, got:\n%s", want, got)
		}
	}
}

func TestFormatResourceRecoveryTitle(t *testing.T) {
	got := FormatResourceAlert(&ResourceAlert{
		ServerName:       "server-1",
		Metric:           "内存",
		Status:           ResourceStatusRecovery,
		UsagePercent:     62,
		ThresholdPercent: 80,
		RecoverPercent:   75,
		ObservedAt:       time.Date(2026, 8, 21, 18, 40, 0, 0, time.UTC),
	})
	if !strings.Contains(got, "[服务器资源恢复]") {
		t.Fatalf("expected recovery title, got:\n%s", got)
	}
}
