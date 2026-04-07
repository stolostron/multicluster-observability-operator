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

# Check whether there is actually anything to tear down. The MCH CR may already
# be gone (e.g. a previous run completed partially) while the operator namespace
# and CSVs are still present, so we check all three independently.
mch_exists=false
oc get multiclusterhub multiclusterhub -n "${ACM_NS}" &>/dev/null && mch_exists=true

# Collect all namespaces that should be gone after a clean teardown.
_acm_namespaces=(
  "${ACM_NS}"
  multicluster-engine
  open-cluster-management-agent
  open-cluster-management-agent-addon
  open-cluster-management-policies
  hypershift
)
any_ns_exists=false
for _ns in "${_acm_namespaces[@]}"; do
  if oc get namespace "$_ns" &>/dev/null; then
    any_ns_exists=true
    break
  fi
done

any_cr_exists=false
oc get klusterlet &>/dev/null 2>&1 && any_cr_exists=true

if ! $mch_exists && ! $any_ns_exists && ! $any_cr_exists; then
  log_info "Nothing to tear down."
  exit 0
fi

if $mch_exists; then
  # Check whether a previous deletion attempt already set a deletionTimestamp.
  # If it did, the finalizer cleanup stalled — skip re-issuing the delete and the
  # wait, and proceed directly to the forced cluster-scoped cleanup below.
  mch_deletion_ts=$(oc get multiclusterhub multiclusterhub -n "${ACM_NS}" \
    -o jsonpath='{.metadata.deletionTimestamp}' 2>/dev/null || true)

  if [[ -n $mch_deletion_ts ]]; then
    log_warn "MultiClusterHub already has deletionTimestamp=${mch_deletion_ts} — previous deletion stalled."
    reply=n
    if [[ -n ${FORCE_CLEANUP:-} ]]; then
      log_warn "FORCE_CLEANUP set — skipping wait and proceeding with forced cleanup."
      reply=y
    elif [[ -t 0 ]]; then
      read -r -p "Skip the wait and proceed with forced cleanup? [y/N] " reply
    else
      log_warn "Non-interactive mode — defaulting to wait. Set FORCE_CLEANUP=1 to skip."
    fi
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
else
  log_info "MultiClusterHub CR already gone, skipping MCH deletion."
fi

# Remove cluster-scoped artifacts that can block namespace deletion if the MCH
# operator did not fully clean them up (e.g. on an aborted previous teardown).
# These are safe to delete unconditionally — they are recreated on reinstall.
log_info "Removing cluster-scoped MCE/ACM artifacts..."
oc delete apiservice \
  v1.admission.cluster.open-cluster-management.io \
  v1.admission.work.open-cluster-management.io \
  --ignore-not-found
# Validating webhooks can block namespace deletion if the underlying service is gone.
oc delete validatingwebhookconfiguration \
  multiclusterengines.multicluster.openshift.io \
  multiclusterhub.validating-webhook.open-cluster-management.io \
  --ignore-not-found
oc delete mce --all --ignore-not-found --wait=false

# Delete the Klusterlet CR while the ACM operator is still running so the
# klusterlet-operator can process its finalizer and clean up the agent namespaces.
# The klusterlet operator lives in the ACM namespace whose CSV we remove later.
log_info "Removing Klusterlet..."
oc delete klusterlet --all --ignore-not-found --wait=false

# The MCE finalizer waits for all ManagedClusterAddOn resources to be removed.
# The hypershift-addon pre-delete hook works by deploying a cleanup Job via
# ManifestWork, which requires the work agent (klusterlet) to be running.
# At this point the klusterlet is already down, so the Job can never execute,
# the ManifestWork never reports completion, and the finalizer never clears.
# Force-remove finalizers from any ManagedClusterAddOn still pending deletion.
log_info "Removing stuck addon-pre-delete finalizers from ManagedClusterAddOns..."
if command -v jq &>/dev/null; then
  while IFS=/ read -r ns name; do
    [[ -z $name ]] && continue
    log_info "  Removing finalizers from ManagedClusterAddOn ${ns}/${name}..."
    oc patch managedclusteraddon "$name" -n "$ns" \
      --type=merge -p '{"metadata":{"finalizers":null}}' 2>/dev/null || true
  done < <(oc get managedclusteraddon --all-namespaces -o json 2>/dev/null |
    jq -r '.items[] | select(.metadata.deletionTimestamp != null) | "\(.metadata.namespace)/\(.metadata.name)"')
else
  log_warn "jq not available — skipping addon finalizer cleanup; MCE deletion may stall"
fi

wait_for_deletion mce multiclusterengine "" 120

# If the klusterlet operator is already gone (e.g. forced teardown path), the
# Klusterlet finalizer will never clear on its own — strip it so the agent
# namespaces are not left stuck in Terminating.
if command -v jq &>/dev/null; then
  while IFS= read -r name; do
    [[ -z $name ]] && continue
    log_info "  Removing finalizers from Klusterlet ${name}..."
    oc patch klusterlet "$name" \
      --type=merge -p '{"metadata":{"finalizers":null}}' 2>/dev/null || true
  done < <(oc get klusterlet -o json 2>/dev/null |
    jq -r '.items[] | select(.metadata.deletionTimestamp != null) | .metadata.name')
fi

# MultiClusterHub finalizer can get stuck if the webhook is gone.
if $mch_exists; then
  log_info "Ensuring MultiClusterHub finalizers are cleared..."
  oc patch multiclusterhub multiclusterhub -n "${ACM_NS}" \
    --type=merge -p '{"metadata":{"finalizers":null}}' 2>/dev/null || true
fi

# Now tell OLM to stop managing both operators.
log_info "Removing ACM OLM resources (CSV and Subscription)..."
oc delete csv --all -n "${ACM_NS}" --ignore-not-found
oc delete subscription --all -n "${ACM_NS}" --ignore-not-found

log_info "Removing MCE OLM resources (CSV and Subscription)..."
oc delete csv --all -n multicluster-engine --ignore-not-found
oc delete subscription --all -n multicluster-engine --ignore-not-found

# Delete all ACM-managed namespaces. The agent and addon namespaces are normally
# cleaned up by the klusterlet, but may linger after a forced teardown.
# hypershift is deployed as plain manifests by MCE (no OLM CSV in that namespace).
log_info "Deleting ACM and MCE namespaces..."
oc delete namespace \
  "${ACM_NS}" \
  multicluster-engine \
  open-cluster-management-agent \
  open-cluster-management-agent-addon \
  open-cluster-management-policies \
  hypershift \
  --ignore-not-found

wait_for_deletion namespace "${ACM_NS}" "" 120
wait_for_deletion namespace multicluster-engine "" 120
wait_for_deletion namespace open-cluster-management-agent "" 120
wait_for_deletion namespace open-cluster-management-agent-addon "" 120
wait_for_deletion namespace open-cluster-management-policies "" 60
wait_for_deletion namespace hypershift "" 120

# Final safety check: if namespaces are still stuck in Terminating, force-strip finalizers.
for _ns in "${_acm_namespaces[@]}"; do
  if oc get namespace "$_ns" 2>/dev/null | grep -q Terminating; then
    log_warn "Namespace $_ns is stuck in Terminating. Forcefully stripping finalizers..."
    oc get namespace "$_ns" -o json | jq '.spec.finalizers = []' >"/tmp/finalize-$_ns.json"
    oc replace --raw "/api/v1/namespaces/$_ns/finalize" -f "/tmp/finalize-$_ns.json" &>/dev/null || true
    rm "/tmp/finalize-$_ns.json"
  fi
done

log_info "ACM removed."
