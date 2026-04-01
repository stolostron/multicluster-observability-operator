#!/usr/bin/env bash
# Removes the MultiClusterObservability CR and waits for all observability
# components to be cleaned up. Does NOT touch the MultiClusterHub or ACM.
#
# Usage:
#   ./teardown-observability.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

if ! oc get multiclusterobservability observability &>/dev/null; then
  log_info "No MultiClusterObservability CR found, nothing to do."
  exit 0
fi

log_info "Deleting MultiClusterObservability CR..."
oc delete multiclusterobservability observability

wait_for_no_pods_in_namespace "${MCO_NS}" 300

log_info "MultiClusterObservability removed. MCH and ACM are untouched."
log_info "Re-deploy with: ./setup-observability.sh"
