#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
RUNTIME_DIR=${RUNTIME_DIR:-"$ROOT_DIR/runtime/local-cluster"}

"$ROOT_DIR/scripts/local-cluster-stop.sh"
rm -rf "$RUNTIME_DIR"
