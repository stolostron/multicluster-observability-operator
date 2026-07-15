#!/usr/bin/env bash
# Sets up downstream dev build catalog sources and installs ACM via OLM.
# Requires an acm-d-pull-secret in openshift-marketplace (run add-pull-secret.sh
# first if needed). CatalogSource.spec.secrets references it directly; this
# script also propagates it to the ACM/MCE operand namespaces via
# 'oc secrets link ... --for=pull' once those namespaces exist, since the
# global cluster pull secret can't be relied on (e.g. it's continuously
# reconciled away on HyperShift-hosted clusters).
#
# Usage — rolling latest build for a given version:
#   ACM_VERSION=5.0 MCE_VERSION=5.0 ./setup-downstream-catalog.sh
#
# Usage — pin to a specific downstream build or release:
#   ACM_VERSION=5.0 MCE_VERSION=5.0 \
#   ACM_CATALOG_TAG=5.0.1-DOWNSTREAM-2026-03-30-06-49-38 \
#   MCE_CATALOG_TAG=5.0.1-DOWNSTREAM-2026-03-30-06-49-38 \
#     ./setup-downstream-catalog.sh
#
#   ACM_VERSION=5.0 MCE_VERSION=5.0 \
#   ACM_CATALOG_TAG=v5.0.1 MCE_CATALOG_TAG=v5.0.1 \
#     ./setup-downstream-catalog.sh
#
# After this script completes, run setup-observability.sh to deploy the MCO stack.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

require_env ACM_VERSION MCE_VERSION

log_info "Checking for acm-d-pull-secret in openshift-marketplace..."
if oc get secret acm-d-pull-secret -n openshift-marketplace &>/dev/null; then
  log_info "acm-d-pull-secret found."
else
  log_error "acm-d-pull-secret not found in openshift-marketplace."
  log_error "Run: QUAY_USER=<user> QUAY_TOKEN=<token> ./add-pull-secret.sh"
  log_error "(or, in Konflux CI: QUAY_PULL_SECRET_FILE=<path> ./add-pull-secret.sh)"
  exit 1
fi

# Default catalog tags to the rolling latest build for the given version.
# Override with ACM_CATALOG_TAG / MCE_CATALOG_TAG to pin to a release candidate.
export ACM_CATALOG_TAG="${ACM_CATALOG_TAG:-latest-${ACM_VERSION}}"
export MCE_CATALOG_TAG="${MCE_CATALOG_TAG:-latest-${MCE_VERSION}}"

require_tool envsubst "Install gettext: brew install gettext (macOS) / dnf install gettext (Fedora)"

MANIFESTS="${SCRIPT_DIR}/manifests/catalog/downstream"

log_info "Applying ImageDigestMirrorSet (applied without node reboots)..."
if ! APPLY_ERR=$(oc apply -f "${MANIFESTS}/image-digest-mirror-set.yaml" 2>&1); then
  if echo "${APPLY_ERR}" | grep -q "ValidatingAdmissionPolicy 'mirror'"; then
    log_warn "ImageDigestMirrorSet is managed by the HostedCluster on HyperShift-hosted clusters (rejected here) — relying on imageContentSources passed at cluster creation instead."
  else
    log_error "${APPLY_ERR}"
    exit 1
  fi
fi

log_info "Applying CatalogSources and Subscriptions for ACM ${ACM_VERSION} / MCE ${MCE_VERSION}..."
export ACM_VERSION MCE_VERSION
envsubst <"${MANIFESTS}/catalogsource.yaml" | oc apply -f -

log_info "Propagating acm-d-pull-secret to operand namespaces for image pulls..."
WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT
oc get secret acm-d-pull-secret -n openshift-marketplace -o jsonpath='{.data.\.dockerconfigjson}' |
  base64 -d >"${WORK_DIR}/acm-d-pull-secret.json"
for ns in "${ACM_NS}" multicluster-engine; do
  oc create secret generic acm-d-pull-secret -n "${ns}" \
    --from-file=.dockerconfigjson="${WORK_DIR}/acm-d-pull-secret.json" \
    --type=kubernetes.io/dockerconfigjson \
    --dry-run=client -o yaml | oc apply -f -
done

log_info "Waiting for ACM CSV to appear in ${ACM_NS}..."
# The subscription triggers an InstallPlan; wait for the CRD that signals ACM is installed.
wait_for_resource crd multiclusterhubs.operator.open-cluster-management.io "" 600 || {
  log_error "=== ACM CSV did not appear — dumping catalog/subscription debug info ==="
  echo "--- CatalogSource status ---"
  oc get catalogsource -n openshift-marketplace -o wide
  oc describe catalogsource acm-custom-registry multiclusterengine-catalog -n openshift-marketplace
  echo "--- Catalog/registry pods ---"
  oc get pods -n openshift-marketplace -o wide
  oc describe pods -n openshift-marketplace -l olm.catalogSource=acm-custom-registry
  oc describe pods -n openshift-marketplace -l olm.catalogSource=multiclusterengine-catalog
  echo "--- Subscriptions and InstallPlans ---"
  oc get subscription,installplan -n "${ACM_NS}" -o wide
  oc describe subscription acm-sub -n "${ACM_NS}"
  echo "--- Recent events in openshift-marketplace and ${ACM_NS} ---"
  oc get events -n openshift-marketplace --sort-by='.lastTimestamp' | tail -30
  oc get events -n "${ACM_NS}" --sort-by='.lastTimestamp' | tail -30
  exit 1
}

# multiclusterhub-operator's own Deployment is created by OLM from the CSV,
# before any MultiClusterHub CR exists, so the CR's spec.imagePullSecret
# (which only propagates to components MCH creates afterward) can't reach
# it. Patch its pod template directly with imagePullSecrets instead —
# unlike linking a secret to its service account, this sets the field on
# the pod spec itself and doesn't depend on admission-time SA lookups, and
# patching the template triggers Kubernetes to roll out fresh pods with it
# automatically, even if earlier pods already exist and are failing.
log_info "Waiting for multiclusterhub-operator Deployment to exist..."
until oc get deployment multiclusterhub-operator -n "${ACM_NS}" &>/dev/null; do sleep 5; done

log_info "Patching multiclusterhub-operator Deployment with acm-d-pull-secret..."
oc patch deployment multiclusterhub-operator -n "${ACM_NS}" --type=json \
  -p='[{"op": "add", "path": "/spec/template/spec/imagePullSecrets", "value": [{"name": "acm-d-pull-secret"}]}]'

log_info "Waiting for MultiClusterHub operator webhook to be ready..."
until oc get pod -n "${ACM_NS}" -l name=multiclusterhub-operator --no-headers 2>/dev/null | grep -q .; do sleep 5; done
oc wait pod -n "${ACM_NS}" -l name=multiclusterhub-operator \
  --for=condition=Ready --timeout=300s || {
  log_error "=== multiclusterhub-operator did not become Ready — dumping debug info ==="
  echo "--- Pod status ---"
  oc get pods -n "${ACM_NS}" -l name=multiclusterhub-operator -o wide
  echo "--- Pod imagePullSecrets (verifying the Deployment patch actually applied) ---"
  oc get pods -n "${ACM_NS}" -l name=multiclusterhub-operator \
    -o jsonpath='{range .items[*]}{.metadata.name}{"  imagePullSecrets="}{.spec.imagePullSecrets}{"\n"}{end}'
  oc get deployment multiclusterhub-operator -n "${ACM_NS}" \
    -o jsonpath='deployment imagePullSecrets={.spec.template.spec.imagePullSecrets}{"\n"}'
  echo "--- Pod description (image, container statuses, events) ---"
  oc describe pods -n "${ACM_NS}" -l name=multiclusterhub-operator
  echo "--- Container logs (current) ---"
  oc logs -n "${ACM_NS}" -l name=multiclusterhub-operator --all-containers --tail=100 || true
  echo "--- Container logs (previous, if crash-looping) ---"
  oc logs -n "${ACM_NS}" -l name=multiclusterhub-operator --all-containers --tail=100 --previous || true
  echo "--- Recent events in ${ACM_NS} ---"
  oc get events -n "${ACM_NS}" --sort-by='.lastTimestamp' | tail -30
  exit 1
}

log_info "Creating MultiClusterHub CR..."
oc apply -f "${SCRIPT_DIR}/manifests/acm/multiclusterhub-cr.yaml"

wait_for_mch_running 900

log_info "ACM is installed and running. Next step: run setup-observability.sh"
