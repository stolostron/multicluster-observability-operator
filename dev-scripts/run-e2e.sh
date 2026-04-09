#!/usr/bin/env bash
# Thin wrapper around dev-scripts/cmd/run-e2e.
# See that package for full documentation and flag reference.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec go run "${SCRIPT_DIR}/cmd/run-e2e" "$@"
