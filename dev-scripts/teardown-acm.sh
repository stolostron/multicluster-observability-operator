#!/usr/bin/env bash
# Fully removes ACM and MCE following the OLM uninstall sequence:
#   1. Delete the MCH CR and wait for the operator to clean up all managed resources.
#   2. Delete ACM and MCE CSVs and Subscriptions so OLM stops managing the operators.
#   3. Delete the namespaces (cascades to OperatorGroups and any remaining artifacts).
#
# Run teardown-observability.sh first if MCO is still deployed — MCH deletion will
# hang while the MCO stack is still running.
#
# Usage:
#   ./teardown-acm.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

# Guard: refuse to proceed if MCO is still deployed.
if oc get multiclusterobservability observability &>/dev/null; then
  log_error "MultiClusterObservability CR still exists. Run ./teardown-observability.sh first."
  exit 1
fi

if ! oc get multiclusterhub multiclusterhub -n "${ACM_NS}" &>/dev/null; then
  log_info "No MultiClusterHub found, nothing to do."
  exit 0
fi

# Check whether a previous deletion attempt already set a deletionTimestamp.
# If it did, the finalizer cleanup stalled — skip re-issuing the delete and the
# wait, and proceed directly to the forced cluster-scoped cleanup below.
mch_deletion_ts=$(oc get multiclusterhub multiclusterhub -n "${ACM_NS}" \
  -o jsonpath='{.metadata.deletionTimestamp}' 2>/dev/null || true)

if [[ -n $mch_deletion_ts ]]; then
  log_warn "MultiClusterHub already has deletionTimestamp=${mch_deletion_ts} — previous deletion stalled."
  read -r -p "Skip the wait and proceed with forced cleanup? [y/N] " reply
  if [[ $reply =~ ^[Yy]$ ]]; then
    log_warn "Skipping wait; proceeding with forced cleanup of cluster-scoped artifacts."
  else
    # Deletion is already in progress — just wait for the finalizer to complete.
    wait_for_mch_deleted 600
  fi
else
  log_info "Deleting MultiClusterHub CR (letting the operator run finalizer cleanup)..."
  oc delete multiclusterhub multiclusterhub -n "${ACM_NS}" --wait=false

  wait_for_mch_deleted 600
fi

# Remove cluster-scoped artifacts that can block namespace deletion if the MCH
# operator did not fully clean them up (e.g. on an aborted previous teardown).
# These are safe to delete unconditionally — they are recreated on reinstall.
log_info "Removing cluster-scoped MCE artifacts..."
oc delete apiservice \
  v1.admission.cluster.open-cluster-management.io \
  v1.admission.work.open-cluster-management.io \
  --ignore-not-found
oc delete validatingwebhookconfiguration multiclusterengines.multicluster.openshift.io \
  --ignore-not-found
oc delete mce --all --ignore-not-found

# Now tell OLM to stop managing both operators.
log_info "Removing ACM OLM resources (CSV and Subscription)..."
oc delete csv --all -n "${ACM_NS}" --ignore-not-found
oc delete subscription --all -n "${ACM_NS}" --ignore-not-found

log_info "Removing MCE OLM resources (CSV and Subscription)..."
oc delete csv --all -n multicluster-engine --ignore-not-found
oc delete subscription --all -n multicluster-engine --ignore-not-found

log_info "Deleting ACM and MCE namespaces..."
oc delete namespace "${ACM_NS}" multicluster-engine --ignore-not-found

wait_for_deletion namespace "${ACM_NS}" "" 120
wait_for_deletion namespace multicluster-engine "" 120

log_info "ACM removed."
