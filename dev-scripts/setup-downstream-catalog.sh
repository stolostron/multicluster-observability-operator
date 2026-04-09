#!/usr/bin/env bash
# Sets up downstream dev build catalog sources and installs ACM via OLM.
# Requires the cluster pull secret to already include quay.io:443/acm-d credentials
# (run add-pull-secret.sh first if needed — many dev clusters already have them).
#
# Usage — rolling latest build for a given version:
#   ACM_VERSION=2.16 MCE_VERSION=2.11 ./setup-downstream-catalog.sh
#
# Usage — pin to a specific downstream build or release:
#   ACM_VERSION=2.16 MCE_VERSION=2.11 \
#   ACM_CATALOG_TAG=2.16.1-DOWNSTREAM-2026-03-30-06-49-38 \
#   MCE_CATALOG_TAG=2.11.1-DOWNSTREAM-2026-03-30-06-49-38 \
#     ./setup-downstream-catalog.sh
#
#   ACM_VERSION=2.16 MCE_VERSION=2.11 \
#   ACM_CATALOG_TAG=v2.16.1 MCE_CATALOG_TAG=v2.11.1 \
#     ./setup-downstream-catalog.sh
#
# After this script completes, run setup-observability.sh to deploy the MCO stack.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

require_env ACM_VERSION MCE_VERSION

# Default catalog tags to the rolling latest build for the given version.
# Override with ACM_CATALOG_TAG / MCE_CATALOG_TAG to pin to a release candidate.
export ACM_CATALOG_TAG="${ACM_CATALOG_TAG:-latest-${ACM_VERSION}}"
export MCE_CATALOG_TAG="${MCE_CATALOG_TAG:-latest-${MCE_VERSION}}"

require_tool envsubst "Install gettext: brew install gettext (macOS) / dnf install gettext (Fedora)"

MANIFESTS="${SCRIPT_DIR}/manifests/catalog/downstream"

log_info "Applying ImageDigestMirrorSet (applied without node reboots)..."
oc apply -f "${MANIFESTS}/image-digest-mirror-set.yaml"

log_info "Applying CatalogSources and Subscriptions for ACM ${ACM_VERSION} / MCE ${MCE_VERSION}..."
export ACM_VERSION MCE_VERSION
envsubst <"${MANIFESTS}/catalogsource.yaml" | oc apply -f -

log_info "Waiting for ACM CSV to appear in ${ACM_NS}..."
# The subscription triggers an InstallPlan; wait for the CRD that signals ACM is installed.
wait_for_resource crd multiclusterhubs.operator.open-cluster-management.io "" 600

log_info "Waiting for MultiClusterHub operator webhook to be ready..."
until oc get pod -n "${ACM_NS}" -l name=multiclusterhub-operator --no-headers 2>/dev/null | grep -q .; do sleep 5; done
oc wait pod -n "${ACM_NS}" -l name=multiclusterhub-operator \
  --for=condition=Ready --timeout=300s

log_info "Creating MultiClusterHub CR..."
oc apply -f "${SCRIPT_DIR}/manifests/acm/multiclusterhub-cr.yaml"

wait_for_mch_running 900

log_info "ACM is installed and running. Next step: run setup-observability.sh"
