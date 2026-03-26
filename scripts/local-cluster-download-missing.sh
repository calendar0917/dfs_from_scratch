#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
RUNTIME_DIR=${RUNTIME_DIR:-"$ROOT_DIR/runtime/local-cluster-download-missing"}
TARGET_FILE=${TARGET_FILE:-"missing-demo.txt"}
MASTER_ADDR=${MASTER_ADDR:-"localhost:50051"}

trap 'RUNTIME_DIR="$RUNTIME_DIR" "$ROOT_DIR/scripts/local-cluster-stop.sh" >/dev/null 2>&1 || true' EXIT

RUNTIME_DIR="$RUNTIME_DIR" "$ROOT_DIR/scripts/local-cluster-clean.sh"
RUNTIME_DIR="$RUNTIME_DIR" "$ROOT_DIR/scripts/local-cluster-start.sh"

set +e
output=$("$ROOT_DIR/bin/client" \
  -action=download \
  -file="$TARGET_FILE" \
  -path="$RUNTIME_DIR/tmp/should-not-exist.txt" \
  -master="$MASTER_ADDR" 2>&1)
status=$?
set -e

if [[ $status -eq 0 ]]; then
  echo "expected download to fail for missing file" >&2
  exit 1
fi

printf '%s\n' "$output"
if [[ "$output" != *"文件不存在"* ]]; then
  echo "expected missing-file error message" >&2
  exit 1
fi

echo "missing-file download test passed"
