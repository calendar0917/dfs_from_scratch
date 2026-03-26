#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
RUNTIME_DIR=${RUNTIME_DIR:-"$ROOT_DIR/runtime/local-cluster-master-restart"}
DATA_DIR="$RUNTIME_DIR/data"
LOG_DIR="$RUNTIME_DIR/logs"
PID_DIR="$RUNTIME_DIR/pids"
TMP_DIR="$RUNTIME_DIR/tmp"
SOURCE_FILE="$TMP_DIR/persist-source.txt"
DOWNLOADED_FILE="$TMP_DIR/persist-downloaded.txt"
FILENAME="persist-demo.txt"
MASTER_ADDR=${MASTER_ADDR:-"localhost:50051"}
MASTER_PORT=${MASTER_PORT:-"50051"}
PERSIST_PATH="$DATA_DIR/master/persist.json"
WAIT_FOR_PERSIST_SECONDS=${WAIT_FOR_PERSIST_SECONDS:-6}

trap 'RUNTIME_DIR="$RUNTIME_DIR" "$ROOT_DIR/scripts/local-cluster-stop.sh" >/dev/null 2>&1 || true' EXIT

RUNTIME_DIR="$RUNTIME_DIR" "$ROOT_DIR/scripts/local-cluster-clean.sh"
mkdir -p "$TMP_DIR"
printf 'persist smoke test\nrun at %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" >"$SOURCE_FILE"
rm -f "$DOWNLOADED_FILE"

RUNTIME_DIR="$RUNTIME_DIR" "$ROOT_DIR/scripts/local-cluster-start.sh"

"$ROOT_DIR/bin/client" \
  -action=upload \
  -file="$FILENAME" \
  -path="$SOURCE_FILE" \
  -master="$MASTER_ADDR"

sleep "$WAIT_FOR_PERSIST_SECONDS"

if [[ ! -f "$PERSIST_PATH" ]]; then
  echo "expected persisted metadata file at $PERSIST_PATH" >&2
  exit 1
fi

master_pid_file="$PID_DIR/master.pid"
if [[ ! -f "$master_pid_file" ]]; then
  echo "missing master pid file" >&2
  exit 1
fi
master_pid=$(cat "$master_pid_file")
kill "$master_pid"
rm -f "$master_pid_file"

nohup "$ROOT_DIR/bin/master" \
  -port="$MASTER_PORT" \
  -persist="$PERSIST_PATH" \
  >"$LOG_DIR/master.log" 2>&1 &
echo $! >"$master_pid_file"

sleep 3
if ! kill -0 "$(cat "$master_pid_file")" 2>/dev/null; then
  echo "master failed to restart" >&2
  cat "$LOG_DIR/master.log" >&2
  exit 1
fi

"$ROOT_DIR/bin/client" \
  -action=download \
  -file="$FILENAME" \
  -path="$DOWNLOADED_FILE" \
  -master="$MASTER_ADDR"

cmp -s "$SOURCE_FILE" "$DOWNLOADED_FILE" || {
  echo "persistence test failed: downloaded content does not match source" >&2
  exit 1
}

echo "master restart persistence test passed"
