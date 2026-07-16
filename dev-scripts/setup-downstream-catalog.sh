#!/usr/bin/env bash
# Sets up downstream dev build catalog sources and installs ACM via OLM.
# Requires an acm-d-pull-secret in openshift-marketplace (run add-pull-secret.sh
# first if needed) — CatalogSource.spec.secrets references it directly for
# the catalog images. Operand image pulls (mirrored via imageContentSources)
# rely on add-pull-secret.sh's additional-pull-secret in kube-system, which
# HyperShift's Global Pull Secret feature syncs to every node's kubelet
# config: https://hypershift.pages.dev/how-to/aws/global-pull-secret/
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
  log_error "(or, in Konflux CI: ./add-pull-secret.sh — no args needed)"
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

log_info "Waiting for MultiClusterHub operator webhook to be ready..."
until oc get pod -n "${ACM_NS}" -l name=multiclusterhub-operator --no-headers 2>/dev/null | grep -q .; do sleep 5; done
oc wait pod -n "${ACM_NS}" -l name=multiclusterhub-operator \
  --for=condition=Ready --timeout=300s || {
  log_error "=== multiclusterhub-operator did not become Ready — dumping debug info ==="
  echo "--- Pod status ---"
  oc get pods -n "${ACM_NS}" -l name=multiclusterhub-operator -o wide
  echo "--- global-pull-secret / syncer state (HyperShift Global Pull Secret feature) ---"
  oc get secret global-pull-secret -n kube-system 2>&1 | head -1
  oc get daemonset global-pull-secret-syncer -n kube-system -o wide 2>&1
  oc get pods -n kube-system -l name=global-pull-secret-syncer -o wide 2>&1
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

wait_for_mch_running 900 || {
  log_error "=== MultiClusterHub did not reach Running — dumping debug info ==="
  echo "--- MultiClusterHub component status ---"
  oc get multiclusterhub multiclusterhub -n "${ACM_NS}" -o json |
    jq '.status.components // {} | to_entries[] | select(.value.type != "Available" or .value.status != "True")' \
    2>/dev/null || oc describe multiclusterhub multiclusterhub -n "${ACM_NS}"
  echo "--- All pods in ${ACM_NS} and multicluster-engine not Running/Completed ---"
  oc get pods -n "${ACM_NS}" -o wide | awk 'NR==1 || $3!="Running"'
  oc get pods -n multicluster-engine -o wide | awk 'NR==1 || $3!="Running"'
  echo "--- Deployments not fully available in ${ACM_NS} and multicluster-engine ---"
  oc get deployments -n "${ACM_NS}" -o wide
  oc get deployments -n multicluster-engine -o wide
  echo "--- Recent events in ${ACM_NS} and multicluster-engine ---"
  oc get events -n "${ACM_NS}" --sort-by='.lastTimestamp' | tail -40
  oc get events -n multicluster-engine --sort-by='.lastTimestamp' | tail -40
  exit 1
}

log_info "ACM is installed and running. Next step: run setup-observability.sh"
