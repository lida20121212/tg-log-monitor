# TG Error Log Monitor

独立 Go 小工具：监控每天分目录下所有 `.log` 文件，只读取新增内容，匹配错误日志后发送到 Telegram 群。

适配你现在的日志目录：

```text
/logs/2026-08-11/app.log
/logs/2026-08-11/server.log
E:\wwwroot\t2_lobby_server\logs\2026-08-10\app.log
```

现有 `t2_lobby_server` 的 zap JSON 日志形如：

```json
{"level":"INFO","time":"2026-08-11T11:55:35.662+0800","caller":"main.go:29","msg":"MySQL connected successfully"}
```

默认匹配 `level=ERROR/FATAL/PANIC` 的 JSON 日志；非 JSON 旧格式匹配单行里的大写 `ERROR/FATAL/PANIC` 或 `panic:`。
同时支持 zap console 格式，例如 `2026-08-21T14:09:34.643+0530	ERROR	paymentchannel/http_request.go:162	...`；日志里带 ANSI 颜色码时会自动清理后再匹配和发送。
默认排除 `"response": {"code":40102`、`"response": {"code":40205`、`"error": "Verification failed", "response": {"code":1,"msg":"Verification failed"`、`This device is already registered...mobile phone number ending in...`、`Field validation for ... failed on the ... tag`，以及 `paymentchannel/`、`payment_channel.go`、`cash_payout.go`、支付/代付回调这类已知三方通道错误，不发送到 TG 群。

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

内网服务器如果不用 systemd，也可以直接用 `deploy.sh` 拉代码并用 `nohup` 常驻：

```bash
cd /www/wwwroot/tg-log-monitor
cp config.example.json config.json   # 只需要首次创建，之后不要覆盖真实配置
vim config.json
bash deploy.sh
```

后续更新只需要：

```bash
cd /www/wwwroot/tg-log-monitor
bash deploy.sh
```

脚本默认会执行 `git pull --ff-only origin main`，构建成功后停掉上一版进程，再用 `nohup` 启动新版。运行信息在 `.runtime/tg-log-monitor.nohup.log`，PID 在 `.runtime/tg-log-monitor.pid`。
如果目录或配置文件名不同，可以用环境变量覆盖：

```bash
APP_DIR=/www/wwwroot/tg-log-monitor CONFIG_FILE=config.json BRANCH=main bash deploy.sh
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
      "file_pattern": "*.log",
      "date_layout": "2006-01-02",
      "timezone": "Asia/Shanghai"
    }
  ],
  "dry_run": false
}
```

同一台服务器如果同时有两种目录，直接在 `sources` 里再加一个对象就行；`date_layout` 留空表示日志直接在 `log_root` 下，不再拼日期目录。

Windows 本地路径示例：

```json
"log_root": "E:\\wwwroot\\t2_lobby_server\\logs"
```

默认会扫描 `log_root/<今天日期>/*.log`；如果 `date_layout` 留空，就会直接扫描 `log_root/*.log`。如果你现在的配置里还有旧字段 `"file_name": "app.log"`，程序会继续只监听这个单文件；要改成监听目录下全部 `.log`，把它换成 `"file_pattern": "*.log"`。
`file_pattern` 也可以写成 `"server-*.log"` 这类更窄的匹配规则。

资源监控只支持 Linux，默认旧配置不写 `resource_monitor` 就不会启用。开启后会轻量采集 CPU、内存、磁盘使用率：

```json
"resource_monitor": {
  "enabled": true,
  "server_name": "server-1",
  "interval": "60s",
  "threshold_percent": 80,
  "recover_percent": 75,
  "cooldown": "10m",
  "notify_recovery": true,
  "disk_paths": ["/", "/www/wwwroot"]
}
```

CPU 使用率按整机总 CPU 容量计算，不是单核 100% 上限。磁盘使用率用 `statfs` 读取路径所在文件系统容量，不会递归扫描目录，也不会执行 `du`。
超过阈值会发告警；持续异常时按 `cooldown` 最多重复提醒一次；降到 `recover_percent` 以下时可发送恢复通知。

也可以用环境变量覆盖：

```bash
export TG_BOT_TOKEN='xxx'
export TG_CHAT_ID='-1001234567890'
export SERVER_NAME='server-1'
export LOG_ROOT='/logs'
./tg-log-monitor -config config.json
```

## 新增日志规则

- 第一次启动时，默认从当前日期目录下已存在的 `.log` 文件末尾开始读，不会把历史错误全部发到群。
- 重启后按 `state.json` 里的 offset 继续读，避免重复推送。
- 进程持续运行跨天时，会自动扫描新的 `YYYY-MM-DD/*.log`；新日期或运行中新出现的 `.log` 文件会从开头读，避免漏掉刚写入的错误。
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
如果 console 错误后面会跟多行堆栈，把 `context_lines_after` 改成 `6` 或 `8` 可以一起带上后续堆栈行。
