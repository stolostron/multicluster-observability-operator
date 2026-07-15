#!/usr/bin/env bash
# Makes quay.io:443/acm-d pull credentials available to the cluster, and
# always ends up with an acm-d-pull-secret in openshift-marketplace so that
# CatalogSource.spec.secrets can reference it unconditionally regardless of
# which path below ran.
#
# Local usage (standard OCP cluster — also patches the global node pull
# secret, so every node can pull mirrored operand images too):
#   QUAY_USER=rh-ee-you QUAY_TOKEN=<token> ./add-pull-secret.sh
#
# Konflux CI usage: these are HyperShift-hosted ephemeral clusters, whose
# openshift-config/pull-secret is continuously reconciled from the
# HostedCluster's spec on the management cluster — direct edits get
# silently reverted within seconds, so we skip the global patch entirely.
# The pipeline's eaas-copy-secrets-to-ephemeral-cluster step already copies
# acm-mco-konflux-e2e from the Konflux tenant namespace into
# openshift-marketplace on this cluster before this script runs; we just
# re-key its "quay.io" auth to also cover "quay.io:443" (the exact host the
# acm-d mirror/catalog images use) and republish it as acm-d-pull-secret.
# setup-downstream-catalog.sh separately propagates acm-d-pull-secret to
# operand namespaces via 'oc secrets link ... --for=pull' once those
# namespaces exist.
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

log_info "Done."
