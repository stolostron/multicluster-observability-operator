#!/usr/bin/env bash
# Removes the MultiClusterObservability CR and waits for all observability
# components to be cleaned up. Does NOT touch the MultiClusterHub or ACM.
#
# Usage:
#   ./teardown-observability.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

mco_exists=false
minio_exists=false
oc get multiclusterobservability observability &>/dev/null && mco_exists=true
oc get deployment minio -n "${MCO_NS}" &>/dev/null && minio_exists=true

if ! $mco_exists && ! $minio_exists; then
  log_info "Nothing to tear down (no MCO CR, no MinIO deployment found)."
  exit 0
fi

if $mco_exists; then
  log_info "Deleting MultiClusterObservability CR..."
  oc delete multiclusterobservability observability
fi

log_info "Deleting MinIO (ephemeral storage deployed by setup-observability.sh)..."
oc delete -f "${SCRIPT_DIR}/manifests/storage/minio-route.yaml" --ignore-not-found
oc delete -f "${SCRIPT_DIR}/manifests/storage/minio-service.yaml" --ignore-not-found
oc delete -f "${SCRIPT_DIR}/manifests/storage/minio-deployment.yaml" --ignore-not-found
oc delete -f "${SCRIPT_DIR}/manifests/storage/thanos-storage-secret.yaml" --ignore-not-found

wait_for_no_pods_in_namespace "${MCO_NS}" 300

log_info "MultiClusterObservability removed. MCH and ACM are untouched."
log_info "Re-deploy with: ./setup-observability.sh"
