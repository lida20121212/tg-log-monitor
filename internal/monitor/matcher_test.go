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
	line := "2026-08-21T13:58:15.532+0530\t\x1b[31mERROR\x1b[0m\tservice/order.go:162\torder request failed\t{\"response_status_code\":502,\"error\":\"backend http status 502\"}"
	if !m.Match(line) {
		t.Fatal("expected colored zap console ERROR to match")
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

func TestDefaultConfigExcludesDeviceAlreadyRegistered(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	m, err := NewMatcher(cfg.Match)
	if err != nil {
		t.Fatal(err)
	}
	line := `{"level":"ERROR","msg":"login request failed","response":{"code":40205,"msg":"This device is already registered. Please log in using your mobile phone number ending in 0415."}}`
	if m.Match(line) {
		t.Fatal("expected device already registered error to be excluded")
	}
}

func TestDefaultConfigExcludesResponseCode40205(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	m, err := NewMatcher(cfg.Match)
	if err != nil {
		t.Fatal(err)
	}
	line := `{"level":"ERROR","msg":"login request failed","response":{"code":40205,"msg":"इस डिवाइस पर पहले से ही एक रजिस्टर्ड अकाउंट है। कृपया अपने मोबाइल फ़ोन नंबर का इस्तेमाल करके लॉग इन करें जिसके आखिर में 7050 हो।"}}`
	if m.Match(line) {
		t.Fatal("expected response code 40205 error to be excluded")
	}
}

func TestDefaultConfigExcludesPaymentChannelErrors(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	m, err := NewMatcher(cfg.Match)
	if err != nil {
		t.Fatal(err)
	}
	line := "2026-08-24T08:10:17.000+0530\tERROR\tpaymentchannel/http_request.go:162\tpayment channel http request failed\t{\"response_status_code\":400,\"error\":\"payment channel http status 400: {\\\"status\\\":\\\"fail\\\",\\\"msg\\\":\\\"sign error\\\"}\"}"
	if m.Match(line) {
		t.Fatal("expected payment channel error to be excluded")
	}
}

func TestDefaultConfigExcludesPaymentChannelGoErrors(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	m, err := NewMatcher(cfg.Match)
	if err != nil {
		t.Fatal(err)
	}
	line := `2026-08-24T11:17:14.046+0530 ERROR service/payment_channel.go:140 create third pay order failed after pay order created {"order_id": 2085893, "order_no": "D1787550433906701", "pay_api": "ROLEXPAY", "provider": "rolexpay", "third_request": {"amount":"500","callback_url":"https://api.apit2game.com/mall/payCallback/rolexpay","merchant_order_no":"D1787550433906701","pay_type":"2","remark":"D1787550433906701","sign":"8981f42c88b2593864fa533a33e5a157","user_id":"10096"}, "third_response": "{\"code\":0,\"msg\":\"支付通道异常：No available channels\",\"time\":\"1787550433\",\"data\":null}", "error": "rolexpay create order failed: code=0 msg=支付通道异常：No available channels"}`
	if m.Match(line) {
		t.Fatal("expected payment_channel.go error to be excluded")
	}
}

func TestDefaultConfigExcludesThirdPayoutOrderErrors(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	m, err := NewMatcher(cfg.Match)
	if err != nil {
		t.Fatal(err)
	}
	line := `2026-08-24T14:36:14.177+0530 ERROR service/cash_payout.go:199 create third payout order failed {"order_id": 306266, "order_no": "W1787562374096391", "uid": 46531504, "pay_api": "HXPAY-huanxing", "provider": "huixin", "third_request": {"account":"924010069782079","amount":500.00,"ifsc":"UTIB0004134","merchantLogin":"T2game","name":"S","notifyUrl":"https://api.apit2game.com/cash/payoutCallback/hxpay-huanxing","orderCode":"W1787562374096391","sign":"344a2600abd469b39d5fbd455e575921"}, "third_response": "{\"code\":\"400\",\"msg\":\"faild\",\"data\":\"请求参数错误：姓名长度不能少于2位\"}", "error": "parse huixin payout response failed"}`
	if m.Match(line) {
		t.Fatal("expected third payout order error to be excluded")
	}
}

func TestDefaultConfigExcludesPayoutCallbackParseErrors(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	m, err := NewMatcher(cfg.Match)
	if err != nil {
		t.Fatal(err)
	}
	line := `2026-08-24T14:36:17.255+0530 ERROR service/cash_payout.go:154 parse payout callback failed {"path":"/cash/payoutCallback/hxpay-huanxing","error":"unexpected end of JSON input"}`
	if m.Match(line) {
		t.Fatal("expected payout callback parse error to be excluded")
	}
}

func TestDefaultConfigExcludesPayoutCallbackErrors(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	m, err := NewMatcher(cfg.Match)
	if err != nil {
		t.Fatal(err)
	}
	line := `2026-08-24T14:36:17.255+0530 ERROR cash/cash.go:146 payout callback failed {"path":"/cash/payoutCallback/hxpay-huanxing","error":"unexpected end of JSON input"}`
	if m.Match(line) {
		t.Fatal("expected payout callback error to be excluded")
	}
}
