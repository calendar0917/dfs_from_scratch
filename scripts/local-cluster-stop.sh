#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
RUNTIME_DIR=${RUNTIME_DIR:-"$ROOT_DIR/runtime/local-cluster"}
PID_DIR="$RUNTIME_DIR/pids"

if [[ ! -d "$PID_DIR" ]]; then
  exit 0
fi

for pid_file in "$PID_DIR"/*.pid; do
  [[ -e "$pid_file" ]] || continue
  pid=$(cat "$pid_file")
  if kill -0 "$pid" 2>/dev/null; then
    kill "$pid" || true
  fi
  rm -f "$pid_file"
done
