#!/usr/bin/env bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -e -o pipefail

SEARCH_DIR=""
SEARCH_FILES=()

if [[ "$(uname)" == "Darwin" ]]; then
  TMP_DIR="${TMPDIR:-/tmp}"
else
  TMP_DIR="/tmp"
fi
OUTPUT_DIR="$TMP_DIR/grafana-dashboards"

# Function to display usage information
usage() {
  echo "Usage: $0 [--directory DIR] [--files FILE1 [FILE2 ...]]"
  echo "Either a directory or at least one file must be specified."
  echo
  echo "Options:"
  echo "  --directory, -d   Specify a directory containing dashboard YAML files"
  echo "  --files, -f       Specify individual dashboard YAML files"
  echo "  --help, -h        Display this help message"
  exit 1
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --directory | -d)
      if [[ -n $SEARCH_DIR ]]; then
        echo "Error: Only one directory can be specified."
        usage
      fi
      shift
      if [[ $# -eq 0 || $1 =~ ^-- ]]; then
        echo "Error: No directory specified after --directory/-d flag."
        usage
      fi
      if [ -d "$1" ]; then
        SEARCH_DIR="$1"
      else
        echo "Error: '$1' is not a valid directory."
        usage
      fi
      shift
      ;;
    --files | -f)
      shift
      while [[ $# -gt 0 && ! $1 =~ ^-- ]]; do
        if [ -f "$1" ]; then
          SEARCH_FILES+=("$1")
        else
          echo "Warning: '$1' is not a valid file. Skipping."
        fi
        shift
      done
      ;;
    --help | -h)
      usage
      ;;
    *)
      echo "Unknown option: $1"
      usage
      ;;
  esac
done

# Check if either directory or files were specified
if [ -z "$SEARCH_DIR" ] && [ ${#SEARCH_FILES[@]} -eq 0 ]; then
  echo "Error: Either a directory or at least one file must be specified."
  usage
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Process directory if specified
if [ -n "$SEARCH_DIR" ]; then
  find "$SEARCH_DIR" -name 'dash*.yaml' ! -name '*ocp311.yaml' -print0 | while IFS= read -r -d '' file; do
    yq '.data | to_entries | .[0].value' "$file" >"$OUTPUT_DIR/$(basename "$file")"
  done
fi

# Process individual files
for file in "${SEARCH_FILES[@]}"; do
  if [[ "$(basename "$file")" == dash*.yaml && "$(basename "$file")" != *ocp311.yaml ]]; then
    yq '.data | to_entries | .[0].value' "$file" >"$OUTPUT_DIR/$(basename "$file")"
  else
    echo "Skipping $file (doesn't match filename pattern)"
  fi
done

# Process the extracted dashboards
file_count=$(find "$OUTPUT_DIR" -type f | wc -l)
if [ "$file_count" -gt 0 ]; then
  files=$(find "$OUTPUT_DIR" -type f -print0 | xargs -0)
  mimirtool analyze dashboard $files --output "$OUTPUT_DIR/dashboards-metrics.json"
  cat "$OUTPUT_DIR/dashboards-metrics.json" | jq -r '.metricsUsed.[]'
else
  echo "No dashboard files were found or processed."
fi

# Clean up
rm -rf "$OUTPUT_DIR"
