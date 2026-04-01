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

log_info "Creating ACM namespace and OperatorGroup..."
oc apply -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: ${ACM_NS}
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: default
  namespace: ${ACM_NS}
spec:
  targetNamespaces:
    - ${ACM_NS}
EOF

log_info "Creating ACM subscription (channel: release-${ACM_VERSION})..."
oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: acm-sub
  namespace: ${ACM_NS}
spec:
  channel: release-${ACM_VERSION}
  installPlanApproval: Automatic
  name: advanced-cluster-management
  source: redhat-operators
  sourceNamespace: openshift-marketplace
EOF

wait_for_resource crd multiclusterhubs.operator.open-cluster-management.io "" 600

log_info "Waiting for MultiClusterHub operator webhook to be ready..."
oc wait pod -n "${ACM_NS}" -l name=multiclusterhub-operator \
  --for=condition=Ready --timeout=300s

log_info "Creating MultiClusterHub CR..."
oc apply -f "${SCRIPT_DIR}/manifests/multiclusterhub-cr.yaml"

wait_for_mch_running 900

log_info "ACM is installed and running. Next step: run setup-observability.sh"
