#!/usr/bin/env bash

# Copyright (c) 2026 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
# Licensed under the Apache License 2.0

set -euo pipefail

# Set default OBO version if not provided
VERSION="${OBO_VERSION:-v0.90.1-rhobs1}"
echo "Using obo-prometheus-operator version: ${VERSION}"

# Base URL for the raw CRD files on GitHub
BASE_URL="https://raw.githubusercontent.com/rhobs/obo-prometheus-operator/refs/tags/${VERSION}/example/prometheus-operator-crd"

# Output directory relative to the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="${SCRIPT_DIR}/../operators/endpointmetrics/controllers/mcoa/crds"

mkdir -p "${OUT_DIR}"

CRDS=(
  "monitoring.rhobs_podmonitors.yaml"
  "monitoring.rhobs_probes.yaml"
  "monitoring.rhobs_prometheusagents.yaml"
  "monitoring.rhobs_prometheuses.yaml"
  "monitoring.rhobs_prometheusrules.yaml"
  "monitoring.rhobs_scrapeconfigs.yaml"
  "monitoring.rhobs_servicemonitors.yaml"
)

# Create a temporary directory for staging downloads
TMP_DIR=$(mktemp -d)
cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

for crd in "${CRDS[@]}"; do
  echo "Downloading ${crd}..."
  if ! curl -sSL -f "${BASE_URL}/${crd}" -o "${TMP_DIR}/${crd}"; then
    echo "Error: Failed to download ${crd} from ${BASE_URL}/${crd}" >&2
    exit 1
  fi
done

# Copy staged CRDs to target directory on success
for crd in "${CRDS[@]}"; do
  cp "${TMP_DIR}/${crd}" "${OUT_DIR}/${crd}"
done

echo "OBO CRDs downloaded successfully to ${OUT_DIR}"
