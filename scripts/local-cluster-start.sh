#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
RUNTIME_DIR=${RUNTIME_DIR:-"$ROOT_DIR/runtime/local-cluster"}
LOG_DIR="$RUNTIME_DIR/logs"
PID_DIR="$RUNTIME_DIR/pids"
DATA_DIR="$RUNTIME_DIR/data"
MASTER_ADDR=${MASTER_ADDR:-"localhost:50051"}
MASTER_PORT=${MASTER_PORT:-"50051"}
VOLUME_NODE_IDS=${VOLUME_NODE_IDS:-"vol-1 vol-2 vol-3"}

mkdir -p "$LOG_DIR" "$PID_DIR" "$DATA_DIR/master"
for node_id in $VOLUME_NODE_IDS; do
  mkdir -p "$DATA_DIR/$node_id"
done

echo "building binaries for local cluster"
make -C "$ROOT_DIR" build

RUNTIME_DIR="$RUNTIME_DIR" "$ROOT_DIR/scripts/local-cluster-stop.sh" >/dev/null 2>&1 || true

nohup "$ROOT_DIR/bin/master" \
  -port="$MASTER_PORT" \
  -persist="$DATA_DIR/master/persist.json" \
  >"$LOG_DIR/master.log" 2>&1 &
echo $! >"$PID_DIR/master.pid"

start_volume() {
  local node_id=$1
  local port=$2
  nohup "$ROOT_DIR/bin/volume" \
    -id="$node_id" \
    -port="$port" \
    -master="$MASTER_ADDR" \
    -storage-dir="$DATA_DIR/$node_id" \
    >"$LOG_DIR/$node_id.log" 2>&1 &
  echo $! >"$PID_DIR/$node_id.pid"
}

assert_process_alive() {
  local name=$1
  local pid_file="$PID_DIR/$name.pid"
  if [[ ! -f "$pid_file" ]]; then
    echo "$name failed to start: missing pid file" >&2
    exit 1
  fi

  local pid
  pid=$(cat "$pid_file")
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "$name failed to stay running; see $LOG_DIR/$name.log" >&2
    exit 1
  fi
}

port_for_node() {
  case "$1" in
    vol-1) echo 50052 ;;
    vol-2) echo 50053 ;;
    vol-3) echo 50054 ;;
    *)
      echo "unsupported node id: $1" >&2
      exit 1
      ;;
  esac
}

for node_id in $VOLUME_NODE_IDS; do
  start_volume "$node_id" "$(port_for_node "$node_id")"
done

sleep 3

assert_process_alive master
for node_id in $VOLUME_NODE_IDS; do
  assert_process_alive "$node_id"
done

echo "local cluster started"
echo "runtime dir: $RUNTIME_DIR"
echo "master log:  $LOG_DIR/master.log"
echo "volume nodes: $VOLUME_NODE_IDS"
