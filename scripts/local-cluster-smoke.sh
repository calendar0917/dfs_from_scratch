#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
RUNTIME_DIR=${RUNTIME_DIR:-"$ROOT_DIR/runtime/local-cluster"}
TMP_DIR="$RUNTIME_DIR/tmp"
SOURCE_FILE="$TMP_DIR/smoke-source.txt"
DOWNLOADED_FILE="$TMP_DIR/smoke-downloaded.txt"
FILENAME="smoke-demo.txt"
MASTER_ADDR=${MASTER_ADDR:-"localhost:50051"}

trap '"$ROOT_DIR/scripts/local-cluster-stop.sh" >/dev/null 2>&1 || true' EXIT

"$ROOT_DIR/scripts/local-cluster-clean.sh"
mkdir -p "$TMP_DIR"
printf 'go-dfs smoke test\nrun at %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" >"$SOURCE_FILE"
rm -f "$DOWNLOADED_FILE"
"$ROOT_DIR/scripts/local-cluster-start.sh"

"$ROOT_DIR/bin/client" \
  -action=upload \
  -file="$FILENAME" \
  -path="$SOURCE_FILE" \
  -master="$MASTER_ADDR"

"$ROOT_DIR/bin/client" \
  -action=download \
  -file="$FILENAME" \
  -path="$DOWNLOADED_FILE" \
  -master="$MASTER_ADDR"

cmp -s "$SOURCE_FILE" "$DOWNLOADED_FILE" || {
  echo "smoke test failed: downloaded content does not match source" >&2
  exit 1
}

echo "smoke test passed"
echo "source:      $SOURCE_FILE"
echo "downloaded:  $DOWNLOADED_FILE"
