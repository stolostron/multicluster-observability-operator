#!/bin/bash

set -e -o pipefail

if [ $# -ne 1 ]; then
  echo "Usage: $0 <directory>"
  exit 1
fi

SEARCH_DIR="$1"

if [[ "$(uname)" == "Darwin" ]]; then
  TMP_DIR="${TMPDIR:-/tmp}"
else
  TMP_DIR="/tmp"
fi

OUTPUT_DIR="$TMP_DIR/grafana-dashboards"
mkdir -p "$OUTPUT_DIR"

find "$SEARCH_DIR" -name 'dash*.yaml' ! -name '*ocp311.yaml' -print0 | while IFS= read -r -d '' file; do
  yq '.data | to_entries | .[0].value' "$file" >"$OUTPUT_DIR/$(basename "$file")"
done

files=$(find "$OUTPUT_DIR" -type f -print0 | xargs -0)
mimirtool analyze dashboard $files --output "$OUTPUT_DIR/dashboards-metrics.json"

cat "$OUTPUT_DIR/dashboards-metrics.json" | jq -r '.metricsUsed.[]'

rm -rf "$OUTPUT_DIR"
