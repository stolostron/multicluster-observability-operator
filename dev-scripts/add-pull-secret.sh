#!/usr/bin/env bash
# Makes quay.io pull credentials available to the cluster.
#
# Local usage:
#   QUAY_USER=<user> QUAY_TOKEN=<token> ./add-pull-secret.sh
#
# Konflux CI usage (HyperShift clusters):
#   ./add-pull-secret.sh   # Uses acm-mco-konflux-e2e secret, no args needed
#
# On HyperShift, uses Global Pull Secret feature (additional-pull-secret in
# kube-system) since openshift-config/pull-secret is reconciled from the
# management cluster. See: https://hypershift.pages.dev/how-to/aws/global-pull-secret/

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

log_info "Checking for acm-mco-konflux-e2e in openshift-marketplace (Konflux CI)..."
COPIED_SECRET_FOUND=0
for _ in $(seq 1 10); do
  if oc get secret acm-mco-konflux-e2e -n openshift-marketplace &>/dev/null; then
    COPIED_SECRET_FOUND=1
    break
  fi
  sleep 10
done

if [[ $COPIED_SECRET_FOUND -eq 1 ]]; then
  require_tool jq
  log_info "Found acm-mco-konflux-e2e in openshift-marketplace."
  oc get secret acm-mco-konflux-e2e -n openshift-marketplace \
    -o jsonpath='{.data.\.dockerconfigjson}' |
    base64 -d >"${WORK_DIR}/copied-secret.json"
  QUAY_AUTH=$(jq -r '.auths["quay.io:443"].auth // .auths["quay.io"].auth // empty' "${WORK_DIR}/copied-secret.json")
  if [[ -z $QUAY_AUTH ]]; then
    log_error "No quay.io auth entry found in acm-mco-konflux-e2e"
    exit 1
  fi
  # Create secret with BOTH quay.io and quay.io:443 entries to cover:
  # - quay.io:443/acm-d/* (downstream dev catalog images)
  # - quay.io/redhat-user-workloads/* (Konflux snapshot images)
  jq -n --arg auth "${QUAY_AUTH}" '{"auths": {"quay.io": {"auth": $auth, "email": ""}, "quay.io:443": {"auth": $auth, "email": ""}}}' \
    >"${WORK_DIR}/pull-secret.json"
else
  log_warn "acm-mco-konflux-e2e not found in openshift-marketplace after waiting; falling back to QUAY_USER/QUAY_TOKEN."
  require_env QUAY_USER QUAY_TOKEN

  log_info "Fetching existing cluster pull secret..."
  oc get secret pull-secret -n openshift-config \
    -o jsonpath='{.data.\.dockerconfigjson}' |
    base64 -d >"${WORK_DIR}/pull-secret.json"

  log_info "Merging quay.io credentials for user ${QUAY_USER}..."
  oc registry login \
    --registry="quay.io:443" \
    --auth-basic="${QUAY_USER}:${QUAY_TOKEN}" \
    --to="${WORK_DIR}/pull-secret.json"

  log_info "Updating cluster pull secret..."
  oc set data secret/pull-secret \
    -n openshift-config \
    --from-file=.dockerconfigjson="${WORK_DIR}/pull-secret.json"
  log_info "Nodes will restart rolling to apply the new credentials — this may take several minutes."
fi

log_info "Creating acm-d-pull-secret in openshift-marketplace for CatalogSource.spec.secrets..."
oc create secret generic acm-d-pull-secret -n openshift-marketplace \
  --from-file=.dockerconfigjson="${WORK_DIR}/pull-secret.json" \
  --type=kubernetes.io/dockerconfigjson \
  --dry-run=client -o yaml | oc apply -f -

if [[ $COPIED_SECRET_FOUND -eq 1 ]]; then
  log_info "Creating additional-pull-secret in kube-system (HyperShift Global Pull Secret feature)..."
  oc create secret generic additional-pull-secret -n kube-system \
    --from-file=.dockerconfigjson="${WORK_DIR}/pull-secret.json" \
    --type=kubernetes.io/dockerconfigjson \
    --dry-run=client -o yaml | oc apply -f -

  # The sync DaemonSet's node label is only auto-applied for AWS/Azure
  # "Replace" NodePools, not "InPlace" ones. Label nodes ourselves in case
  # this cluster's NodePool doesn't get it automatically — best-effort,
  # since forcing this on an InPlace NodePool isn't an officially
  # documented combination.
  oc label nodes --all hypershift.openshift.io/nodepool-globalps-enabled=true --overwrite 2>/dev/null || true

  log_info "Waiting for HCCO to merge additional-pull-secret into global-pull-secret..."
  GLOBAL_SECRET_FOUND=0
  for _ in $(seq 1 30); do
    if oc get secret global-pull-secret -n kube-system &>/dev/null; then
      GLOBAL_SECRET_FOUND=1
      break
    fi
    sleep 10
  done
  if [[ $GLOBAL_SECRET_FOUND -eq 1 ]]; then
    log_info "global-pull-secret created; global-pull-secret-syncer will sync it to eligible nodes."
  else
    log_error "global-pull-secret did not appear after 5 minutes — HCCO failed to merge additional-pull-secret."
    exit 1
  fi
fi

log_info "Done."
