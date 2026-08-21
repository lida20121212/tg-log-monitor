# TG Error Log Monitor

独立 Go 小工具：监控每天分目录的 `app.log`，只读取新增内容，匹配错误日志后发送到 Telegram 群。

适配你现在的日志目录：

```text
/logs/2026-08-11/app.log
E:\wwwroot\t2_lobby_server\logs\2026-08-10\app.log
```

现有 `t2_lobby_server` 的 zap JSON 日志形如：

```json
{"level":"INFO","time":"2026-08-11T11:55:35.662+0800","caller":"main.go:29","msg":"MySQL connected successfully"}
```

默认匹配 `level=ERROR/FATAL/PANIC` 的 JSON 日志；非 JSON 旧格式匹配单行里的大写 `ERROR/FATAL/PANIC` 或 `panic:`。
默认排除 `"response": {"code":40102` 和 `"error": "Verification failed", "response": {"code":1,"msg":"Verification failed"` 这类已知业务错误，不发送到 TG 群。

## 使用

```powershell
cd E:\wwwroot\tg-log-monitor
copy config.example.json config.json
go build -buildvcs=false -o tg-log-monitor.exe .
.\tg-log-monitor.exe -config config.json
```

如果只是本地源码调试，现在可以直接运行入口文件：

```powershell
go run main.go -config .\config.example.json
```

Linux 服务器：

```bash
cd /opt/tg-log-monitor
cp config.example.json config.json
go build -buildvcs=false -o tg-log-monitor .
./tg-log-monitor -config config.json
```

先测试 TG bot 和 chat_id 是否可用：

```bash
./tg-log-monitor -config config.json -test-telegram
```

## 配置

`config.example.json` 已带中文注释，程序支持 `//` 和 `/* */` 注释；复制成 `config.json` 后可以直接保留注释运行。

两台服务器建议各自部署一份，只改 `name` 区分来源：

```json
{
  "telegram": {
    "bot_token": "你的 bot token",
    "chat_id": "-100xxxxxxxxxx",
    "parse_mode": ""
  },
  "sources": [
    {
      "name": "server-1",
      "log_root": "/logs",
      "file_name": "app.log",
      "date_layout": "2006-01-02",
      "timezone": "Asia/Shanghai"
    }
  ],
  "dry_run": false
}
```

Windows 本地路径示例：

```json
"log_root": "E:\\wwwroot\\t2_lobby_server\\logs"
```

也可以用环境变量覆盖：

```bash
export TG_BOT_TOKEN='xxx'
export TG_CHAT_ID='-1001234567890'
export SERVER_NAME='server-1'
export LOG_ROOT='/logs'
./tg-log-monitor -config config.json
```

## 新增日志规则

- 第一次启动时，默认从当前 `app.log` 文件末尾开始读，不会把历史错误全部发到群。
- 重启后按 `state.json` 里的 offset 继续读，避免重复推送。
- 进程持续运行跨天时，会自动切到新的 `YYYY-MM-DD/app.log`；新日期文件会从开头读，避免漏掉凌晨刚写入的错误。
- 如果你想首次启动就扫描已有文件，把 `start_at_end` 改成 `false`。
- TG 连续失败两次后会熔断 1 分钟；队列满时会丢弃尾部告警，避免监控进程被远端故障拖住。

如果启动后看到 `watching ... from offset 10290962`，表示会从这个 offset 后面继续读，之前已有的历史错误不会再发送。实时测试可以在监控进程运行时追加一条新 ERROR：

```bash
printf '%s\tERROR\ttg-test/manual.go:1\ttg monitor live test\t{"error":"manual test"}\n' "$(date '+%Y-%m-%dT%H:%M:%S.000%z')" >> /www/wwwroot/t2_lobby_server/logs/$(date +%F)/app.log
```

## systemd

```bash
sudo mkdir -p /opt/tg-log-monitor
sudo cp tg-log-monitor config.json /opt/tg-log-monitor/
sudo cp deploy/tg-log-monitor.service /etc/systemd/system/tg-log-monitor.service
sudo systemctl daemon-reload
sudo systemctl enable --now tg-log-monitor
sudo journalctl -u tg-log-monitor -f
```

补充日志格式后，可以把 `include_regex` 收紧到你的真实错误字段，减少误报。
