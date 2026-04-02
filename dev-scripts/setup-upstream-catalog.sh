#!/usr/bin/env bash
# Installs ACM from the standard redhat-operators OLM catalog on a fresh cluster,
# then creates the MultiClusterHub CR and waits for it to reach Running state.
#
# Use this when the cluster has no ACM installed and you don't need downstream
# dev builds — just a clean ACM install to test against.
#
# Usage:
#   ACM_VERSION=2.16 ./setup-upstream-catalog.sh
#
# After this completes, run setup-observability.sh to deploy the MCO stack.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

require_env ACM_VERSION

require_tool envsubst "Install gettext: brew install gettext (macOS) / dnf install gettext (Fedora)"

MANIFESTS="${SCRIPT_DIR}/manifests"

log_info "Creating ACM namespace, OperatorGroup, and Subscription (channel: release-${ACM_VERSION})..."
envsubst <"${MANIFESTS}/catalog/upstream/subscription.yaml" | oc apply -f -

wait_for_resource crd multiclusterhubs.operator.open-cluster-management.io "" 600

log_info "Waiting for MultiClusterHub operator webhook to be ready..."
until oc get pod -n "${ACM_NS}" -l name=multiclusterhub-operator --no-headers 2>/dev/null | grep -q .; do sleep 5; done
oc wait pod -n "${ACM_NS}" -l name=multiclusterhub-operator \
  --for=condition=Ready --timeout=300s

log_info "Creating MultiClusterHub CR..."
oc apply -f "${MANIFESTS}/acm/multiclusterhub-cr.yaml"

wait_for_mch_running 900

log_info "ACM is installed and running. Next step: run setup-observability.sh"
