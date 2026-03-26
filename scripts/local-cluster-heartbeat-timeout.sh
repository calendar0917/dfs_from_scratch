#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
RUNTIME_DIR=${RUNTIME_DIR:-"$ROOT_DIR/runtime/local-cluster-heartbeat"}
MASTER_LOG="$RUNTIME_DIR/logs/master.log"
PID_DIR="$RUNTIME_DIR/pids"
TARGET_NODE=${TARGET_NODE:-"vol-3"}
WAIT_SECONDS=${WAIT_SECONDS:-13}

trap 'RUNTIME_DIR="$RUNTIME_DIR" "$ROOT_DIR/scripts/local-cluster-stop.sh" >/dev/null 2>&1 || true' EXIT

RUNTIME_DIR="$RUNTIME_DIR" "$ROOT_DIR/scripts/local-cluster-clean.sh"
RUNTIME_DIR="$RUNTIME_DIR" "$ROOT_DIR/scripts/local-cluster-start.sh"

pid_file="$PID_DIR/$TARGET_NODE.pid"
if [[ ! -f "$pid_file" ]]; then
  echo "missing pid file for $TARGET_NODE" >&2
  exit 1
fi

pid=$(cat "$pid_file")
kill "$pid"
rm -f "$pid_file"

echo "stopped $TARGET_NODE, waiting ${WAIT_SECONDS}s for heartbeat timeout"
sleep "$WAIT_SECONDS"

if ! rg -q "节点 ${TARGET_NODE} 心跳超时，执行剔除" "$MASTER_LOG"; then
  echo "expected heartbeat timeout eviction log for $TARGET_NODE" >&2
  echo "master log:" >&2
  cat "$MASTER_LOG" >&2
  exit 1
fi

echo "heartbeat timeout test passed"
