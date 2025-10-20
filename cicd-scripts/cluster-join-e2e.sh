#!/bin/bash

# Utilities for controlling command output
# quiet: run a command with both stdout and stderr suppressed
quiet() { "$@" >/dev/null 2>&1; }

set +x

# Extract hub cluster info from JSON
HUB_API_URL=$(jq -r '.api_url' "${SHARED_DIR}/hub-1.json" 2>/dev/null)
HUB_USER=$(jq -r '.username' "${SHARED_DIR}/hub-1.json" 2>/dev/null)
HUB_PASS=$(jq -r '.password' "${SHARED_DIR}/hub-1.json" 2>/dev/null)
set -x 

for ((i=0 ; i < CLUSTERPOOL_MANAGED_COUNT ; i++)); do
  if [[ -z ${IS_KIND_ENV} ]] && [[ ! -z "${SHARED_DIR}" ]] && [[ -f "${SHARED_DIR}/managed-${i}.kc" ]]; then
    set +x
    {
      MANAGED_CLUSTER_API_URL=$(jq -r '.api_url' "${SHARED_DIR}/managed-${i}.json" 2>/dev/null)
      MANAGED_CLUSTER_USER=$(jq -r '.username' "${SHARED_DIR}/managed-${i}.json" 2>/dev/null)
      MANAGED_CLUSTER_PASS=$(jq -r '.password' "${SHARED_DIR}/managed-${i}.json" 2>/dev/null)
    } >/dev/null 2>&1
    set -x


    # join clusters hub and managed cluster (suppress noisy output)
    quiet clusteradm init || true
    set +x
    # need to supress output here
    HUB_TOKEN=$(clusteradm get token) || true
    # Switch context to managed cluster
    quiet oc login --insecure-skip-tls-verify -u "$MANAGED_CLUSTER_USER" -p "$MANAGED_CLUSTER_PASS" "$MANAGED_CLUSTER_API_URL" || true
    set -x
    quiet clusteradm join --hub-token ${HUB_TOKEN} --hub-api-server ${HUB_API_SERVER} --cluster-name ${MANAGED_CLUSTER_NAME} || true
    set +x
    quiet oc login --insecure-skip-tls-verify -u "$HUB_USER" -p "$HUB_PASS" "$HUB_API_URL" || true
    set -x
    # Set kubeconfig back to hub
    export KUBECONFIG="${SHARED_DIR}/hub-1.kc"
    quiet clusteradm accept --clusters ${MANAGED_CLUSTER_NAME} || true
  fi
done