package monitor

import "testing"

func TestMatcherMatchesZapJSONError(t *testing.T) {
	m, err := NewMatcher(MatchConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match(`{"level":"ERROR","time":"2026-08-11T12:00:00+0800","msg":"failed"}`) {
		t.Fatal("expected JSON ERROR to match")
	}
	if m.Match(`{"level":"INFO","time":"2026-08-11T12:00:00+0800","msg":"ok"}`) {
		t.Fatal("expected JSON INFO not to match")
	}
}

func TestMatcherDoesNotMatchJSONInfoWithErrorText(t *testing.T) {
	m, err := NewMatcher(MatchConfig{IncludeRegex: []string{`\bERROR\b`}})
	if err != nil {
		t.Fatal(err)
	}
	if m.Match(`{"level":"INFO","time":"2026-08-11T12:00:00+0800","msg":"contains ERROR text"}`) {
		t.Fatal("expected JSON INFO with ERROR text not to match")
	}
}

func TestMatcherMatchesFatalAndPanicLevel(t *testing.T) {
	m, err := NewMatcher(MatchConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match(`{"level":"FATAL","time":"2026-08-11T12:00:00+0800","msg":"fatal"}`) {
		t.Fatal("expected JSON FATAL to match")
	}
	if !m.Match(`{"level":"PANIC","time":"2026-08-11T12:00:00+0800","msg":"panic"}`) {
		t.Fatal("expected JSON PANIC to match")
	}
}

func TestMatcherMatchesConsoleError(t *testing.T) {
	m, err := NewMatcher(MatchConfig{IncludeRegex: []string{`\bERROR\b`}})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match(`2026-08-11T12:00:00+0800 ERROR service failed`) {
		t.Fatal("expected console ERROR to match")
	}
}

func TestDefaultMatcherMatchesZapConsoleError(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	m, err := NewMatcher(cfg.Match)
	if err != nil {
		t.Fatal(err)
	}
	line := "2026-08-21T13:58:15.532+0530\tERROR\tpaymentchannel/http_request.go:162\tpayment channel http request failed\t{\"response_status_code\":502,\"error\":\"payment channel http status 502\"}"
	if !m.Match(line) {
		t.Fatal("expected zap console ERROR to match")
	}
}

func TestMatcherExcludeWins(t *testing.T) {
	m, err := NewMatcher(MatchConfig{
		IncludeRegex: []string{`\bERROR\b`},
		ExcludeRegex: []string{`ignore this`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Match(`ERROR ignore this`) {
		t.Fatal("expected excluded line not to match")
	}
}

func TestDefaultConfigExcludesResponseCode40102(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	m, err := NewMatcher(cfg.Match)
	if err != nil {
		t.Fatal(err)
	}
	line := `{"level":"ERROR","msg":"business error","response":{"code":40102,"msg":"token invalid"}}`
	if m.Match(line) {
		t.Fatal("expected response code 40102 error to be excluded")
	}
}

func TestDefaultConfigExcludesResponseCode40102WithSpaces(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	m, err := NewMatcher(cfg.Match)
	if err != nil {
		t.Fatal(err)
	}
	line := `{"level":"ERROR","msg":"business error","response": {"code":40102, "msg":"token invalid"}}`
	if m.Match(line) {
		t.Fatal("expected spaced response code 40102 error to be excluded")
	}
}

func TestDefaultConfigExcludesVerificationFailedCode1(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	m, err := NewMatcher(cfg.Match)
	if err != nil {
		t.Fatal(err)
	}
	line := `{"level":"ERROR","msg":"merchant verify failed","error": "Verification failed", "response": {"code":1,"msg":"Verification failed","data":null}}`
	if m.Match(line) {
		t.Fatal("expected verification failed code 1 error to be excluded")
	}
}
