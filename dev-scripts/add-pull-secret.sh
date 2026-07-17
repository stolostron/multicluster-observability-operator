#!/usr/bin/env bash
# Makes quay.io:443/acm-d pull credentials available to the cluster.
#
# Local usage (standard OCP cluster — patches the global node pull secret
# directly; this persists normally on non-HyperShift clusters):
#   QUAY_USER=rh-ee-you QUAY_TOKEN=<token> ./add-pull-secret.sh
#
# Konflux CI usage: these are HyperShift-hosted ephemeral clusters, whose
# openshift-config/pull-secret is continuously reconciled from the
# HostedCluster's spec on the management cluster — direct edits get
# silently reverted within seconds. Per-pod/SA/Deployment-level secrets
# don't help either: mirror-redirected pulls (e.g. registry.redhat.io ->
# quay.io:443/acm-d via imageContentSources) are resolved by CRI-O at the
# node level and only ever consult the node's own credential file,
# regardless of what's attached to the pod. Proven empirically — a pod
# with acm-d-pull-secret correctly set in its own imagePullSecrets still
# got "unauthorized" pulling a mirrored image.
#
# Instead, use HyperShift's "Global Pull Secret" feature: creating
# additional-pull-secret in kube-system is a dedicated, sanctioned
# extension point (unlike openshift-config/pull-secret) that the Hosted
# Cluster Config Operator merges into global-pull-secret and syncs to
# every eligible node's kubelet config via the global-pull-secret-syncer
# DaemonSet — no management-cluster access needed.
# https://hypershift.pages.dev/how-to/aws/global-pull-secret/
#
# The pipeline's eaas-copy-secrets-to-ephemeral-cluster step already copies
# acm-mco-konflux-e2e from the Konflux tenant namespace into
# openshift-marketplace on this cluster before this script runs.
#   ./add-pull-secret.sh   # no args needed in Konflux CI

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
  jq -n --arg auth "${QUAY_AUTH}" '{"auths": {"quay.io:443": {"auth": $auth, "email": ""}}}' \
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
  QUAY_AUTH_PROPAGATED=0
  for _ in $(seq 1 30); do
    if oc get secret global-pull-secret -n kube-system &>/dev/null; then
      # Verify QUAY_AUTH is actually present in the merged secret, not just that the secret exists
      MERGED_AUTH=$(oc get secret global-pull-secret -n kube-system \
        -o jsonpath='{.data.\.dockerconfigjson}' 2>/dev/null | \
        base64 -d 2>/dev/null | \
        jq -r '.auths["quay.io:443"].auth // .auths["quay.io"].auth // empty' 2>/dev/null || true)
      if [[ -n "$MERGED_AUTH" && "$MERGED_AUTH" == "$QUAY_AUTH" ]]; then
        QUAY_AUTH_PROPAGATED=1
        break
      fi
    fi
    sleep 10
  done
  if [[ $QUAY_AUTH_PROPAGATED -eq 1 ]]; then
    log_info "QUAY_AUTH successfully propagated to global-pull-secret; global-pull-secret-syncer will sync it to eligible nodes."
  else
    log_error "QUAY_AUTH did not appear in global-pull-secret after 5 minutes — HCCO failed to merge additional-pull-secret."
    exit 1
  fi
fi

log_info "Done."
