#!/usr/bin/env bash
# Sets up downstream dev build catalog sources and installs ACM via OLM.
# Requires the cluster pull secret to already include registry.stage.redhat.io credentials
# (run add-pull-secret.sh first if needed — many dev clusters already have them).
#
# Usage — rolling latest build for a given version:
#   ACM_VERSION=5.0 MCE_VERSION=5.0 ./setup-art-catalog.sh
#
# After this script completes, run setup-observability.sh to deploy the MCO stack.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

require_env ACM_VERSION MCE_VERSION

OCP_VERSION=$(oc version | grep "Server Version" | awk '{print $3}' | cut -f1 -d"-")
OCP_VERSION=${OCP_VERSION%.*}

export ACM_CATALOG_TAG="acm-${ACM_VERSION}__v${OCP_VERSION}__multiclusterhub-rhel9"
export MCE_CATALOG_TAG="mce-${MCE_VERSION}__v${OCP_VERSION}__backplane-rhel9-operator"

require_tool envsubst "Install gettext: brew install gettext (macOS) / dnf install gettext (Fedora)"

MANIFESTS="${SCRIPT_DIR}/manifests/catalog/art"

if oc get imagecontentsourcepolicies rhacm-repo &>/dev/null; then
  log_info "Deleting old acm-d ImageConectSourcePolicy"
  oc delete imagecontentsourcepolicies rhacm-repo
fi

log_info "Applying ImageDigestMirrorSet (applied without node reboots)..."
oc apply -f "${MANIFESTS}/image-digest-mirror-set.yaml"

# For spokes we only want to set the IDMS
if [[ -v SPOKE ]]; then
  log_info "IDMS configured for spoke."
  exit 0
fi

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
