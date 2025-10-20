#!/bin/bash

set +x
# Extract hub cluster info from JSON
HUB_API_URL=$(jq -r '.api_url' "${SHARED_DIR}/hub-1.json" &>/dev/null)
HUB_USER=$(jq -r '.username' "${SHARED_DIR}/hub-1.json" &>/dev/null)
HUB_PASS=$(jq -r '.password' "${SHARED_DIR}/hub-1.json" &>/dev/null)
set -x 

for ((i=0 ; i < CLUSTERPOOL_MANAGED_COUNT ; i++)); do
  if [[ -z ${IS_KIND_ENV} ]] && [[ ! -z "${SHARED_DIR}" ]] && [[ -f "${SHARED_DIR}/managed-${i}.kc" ]]; then
    set +x
    MANAGED_CLUSTER_API_URL=$(jq -r '.api_url' "${SHARED_DIR}/managed-${i}.json" &>/dev/null)
    MANAGED_CLUSTER_USER=$(jq -r '.username' "${SHARED_DIR}/managed-${i}.json" &>/dev/null)
    MANAGED_CLUSTER_PASS=$(jq -r '.password' "${SHARED_DIR}/managed-${i}.json" &>/dev/null)
    set -x

    # join clusters hub and managed cluster (suppress noisy output)
    clusteradm init managed-${i} &>/dev/null
    set +x
    # need to supress output here
    HUB_TOKEN=$(clusteradm get token) managed-${i} &>/dev/null
    # Switch context to managed cluster
    oc login --insecure-skip-tls-verify -u "$MANAGED_CLUSTER_USER" -p "$MANAGED_CLUSTER_PASS" "$MANAGED_CLUSTER_API_URL" &>/dev/null
    clusteradm join --hub-token ${HUB_TOKEN} --hub-api-server ${HUB_API_SERVER} --cluster-name ${MANAGED_CLUSTER_NAME} managed-${i} &>/dev/null
    oc login --insecure-skip-tls-verify -u "$HUB_USER" -p "$HUB_PASS" "$HUB_API_URL" managed-${i} &>/dev/null
    set -x
    # Set kubeconfig back to hub
    export KUBECONFIG="${SHARED_DIR}/hub-1.kc"
    clusteradm accept --clusters ${MANAGED_CLUSTER_NAME} managed-${i} &>/dev/null
  fi
done