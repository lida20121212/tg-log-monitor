#!/usr/bin/env bash
set -Eeuo pipefail

# Usage:
#   bash deploy.sh
#
# Optional overrides:
#   APP_DIR=/www/wwwroot/tg-log-monitor CONFIG_FILE=config.json BRANCH=main bash deploy.sh

APP_NAME="${APP_NAME:-tg-log-monitor}"
BRANCH="${BRANCH:-main}"
CONFIG_FILE="${CONFIG_FILE:-config.json}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="${APP_DIR:-$SCRIPT_DIR}"
APP_DIR="$(cd "$APP_DIR" && pwd)"

RUN_DIR="${RUN_DIR:-$APP_DIR/.runtime}"
PID_FILE="${PID_FILE:-$RUN_DIR/$APP_NAME.pid}"
NOHUP_LOG="${NOHUP_LOG:-$RUN_DIR/$APP_NAME.nohup.log}"
BIN_PATH="${BIN_PATH:-$APP_DIR/$APP_NAME}"
BUILD_BIN="$RUN_DIR/$APP_NAME.new"
PREV_BIN="$RUN_DIR/$APP_NAME.prev"

if [[ "$CONFIG_FILE" = /* ]]; then
  CONFIG_PATH="$CONFIG_FILE"
else
  CONFIG_PATH="$APP_DIR/$CONFIG_FILE"
fi

log() {
  printf '[%s] %s\n' "$(date '+%F %T')" "$*"
}

die() {
  log "ERROR: $*"
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

kill_pid() {
  local pid="${1:-}"
  [[ "$pid" =~ ^[0-9]+$ ]] || return 0
  kill -0 "$pid" 2>/dev/null || return 0

  log "stopping previous process pid=$pid"
  kill "$pid" 2>/dev/null || true

  for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
    if ! kill -0 "$pid" 2>/dev/null; then
      log "process stopped pid=$pid"
      return 0
    fi
    sleep 0.5
  done

  log "process still alive, force killing pid=$pid"
  kill -9 "$pid" 2>/dev/null || true
}

is_app_process() {
  local cmd="$1"
  local cwd="$2"

  [[ "$cmd" == *"$BIN_PATH"* ]] && return 0
  [[ "$cwd" == "$APP_DIR" ]] || return 1

  case " $cmd " in
    *" ./$APP_NAME "* | *" $APP_NAME "*) return 0 ;;
    *) return 1 ;;
  esac
}

stop_old_process() {
  if [[ -f "$PID_FILE" ]]; then
    local pid
    pid="$(tr -cd '0-9' <"$PID_FILE" || true)"
    kill_pid "$pid"
    rm -f "$PID_FILE"
  fi

  command -v pgrep >/dev/null 2>&1 || return 0

  local pid cmd cwd
  while IFS= read -r pid; do
    [[ -n "$pid" ]] || continue
    [[ "$pid" == "$$" ]] && continue

    cmd="$(tr '\0' ' ' <"/proc/$pid/cmdline" 2>/dev/null || true)"
    [[ -n "$cmd" ]] || continue

    cwd="$(readlink -f "/proc/$pid/cwd" 2>/dev/null || true)"
    if is_app_process "$cmd" "$cwd"; then
      kill_pid "$pid"
    fi
  done < <(pgrep -f -- "$APP_NAME" || true)
}

pull_code() {
  [[ -d "$APP_DIR/.git" ]] || die "$APP_DIR is not a git repo"
  require_cmd git

  log "pulling latest code from origin/$BRANCH"
  git fetch origin "$BRANCH"
  git pull --ff-only origin "$BRANCH"
}

build_binary() {
  require_cmd go
  mkdir -p "$RUN_DIR"
  rm -f "$BUILD_BIN"

  log "building $APP_NAME"
  go build -buildvcs=false -o "$BUILD_BIN" .
  chmod +x "$BUILD_BIN"
}

replace_binary() {
  if [[ -f "$BIN_PATH" ]]; then
    cp -p "$BIN_PATH" "$PREV_BIN" || true
  fi
  mv -f "$BUILD_BIN" "$BIN_PATH"
  chmod +x "$BIN_PATH"
}

start_process() {
  require_cmd nohup
  touch "$NOHUP_LOG"

  log "starting $APP_NAME with nohup"
  nohup "$BIN_PATH" -config "$CONFIG_PATH" >>"$NOHUP_LOG" 2>&1 &
  local pid="$!"
  printf '%s\n' "$pid" >"$PID_FILE"

  sleep 1
  if kill -0 "$pid" 2>/dev/null; then
    log "started pid=$pid"
    log "pid file: $PID_FILE"
    log "nohup log: $NOHUP_LOG"
    return 0
  fi

  rm -f "$PID_FILE"
  tail -n 50 "$NOHUP_LOG" 2>/dev/null || true
  die "start failed, check $NOHUP_LOG"
}

main() {
  cd "$APP_DIR"
  [[ -f "$CONFIG_PATH" ]] || die "config not found: $CONFIG_PATH"

  pull_code
  build_binary
  stop_old_process
  replace_binary
  start_process
  log "deploy done"
}

main "$@"
