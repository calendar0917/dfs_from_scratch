#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
RUNTIME_DIR=${RUNTIME_DIR:-"$ROOT_DIR/runtime/local-cluster-insufficient"}
TMP_DIR="$RUNTIME_DIR/tmp"
SOURCE_FILE="$TMP_DIR/insufficient-source.txt"
FILENAME="insufficient-demo.txt"
MASTER_ADDR=${MASTER_ADDR:-"localhost:50051"}

trap 'RUNTIME_DIR="$RUNTIME_DIR" "$ROOT_DIR/scripts/local-cluster-stop.sh" >/dev/null 2>&1 || true' EXIT

RUNTIME_DIR="$RUNTIME_DIR" "$ROOT_DIR/scripts/local-cluster-clean.sh"
mkdir -p "$TMP_DIR"
printf 'insufficient nodes test\n' >"$SOURCE_FILE"

if RUNTIME_DIR="$RUNTIME_DIR" VOLUME_NODE_IDS="vol-1 vol-2" "$ROOT_DIR/scripts/local-cluster-start.sh"; then
  echo "started 2-node volume cluster"
else
  echo "failed to start insufficient-nodes cluster" >&2
  exit 1
fi

set +e
output=$("$ROOT_DIR/bin/client" \
  -action=upload \
  -file="$FILENAME" \
  -path="$SOURCE_FILE" \
  -master="$MASTER_ADDR" 2>&1)
status=$?
set -e

if [[ $status -eq 0 ]]; then
  echo "expected upload to fail when fewer than 3 volume nodes are available" >&2
  exit 1
fi

printf '%s\n' "$output"
if [[ "$output" != *"可用节点不足"* ]]; then
  echo "expected insufficient nodes error message" >&2
  exit 1
fi

echo "insufficient nodes test passed"
