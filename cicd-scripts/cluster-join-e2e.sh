#!/bin/bash

# Extract hub cluster info from JSON
HUB_API_URL=$(jq -r '.api_url' "${SHARED_DIR}/hub-1.json") &>/dev/null
HUB_USER=$(jq -r '.username' "${SHARED_DIR}/hub-1.json")
HUB_PASS=$(jq -r '.password' "${SHARED_DIR}/hub-1.json")
echo "HUB_API_URL: ${HUB_API_URL}"
clusteradm init &>/dev/null
HUB_TOKEN=$(clusteradm get token) &>/dev/null
# list of managed clusters
managed_clusters=""
for ((i=1 ; i <= CLUSTERPOOL_MANAGED_COUNT ; i++)); do
  if [[ ! -z "${HUB_TOKEN}" ]] && [[ ! -z "${SHARED_DIR}" ]] && [[ -f "${SHARED_DIR}/managed-${i}.json" ]]; then
    echo "Joining cluster ${i}"
    if [[ $i -eq 1 ]]; then
      managed_clusters="managed-${i}"
    else
      managed_clusters+=",managed-${i}"
    fi
    MANAGED_CLUSTER_API_URL=$(jq -r '.api_url' "${SHARED_DIR}/managed-${i}.json" )
    MANAGED_CLUSTER_USER=$(jq -r '.username' "${SHARED_DIR}/managed-${i}.json" )
    MANAGED_CLUSTER_PASS=$(jq -r '.password' "${SHARED_DIR}/managed-${i}.json" )
    echo "MANAGED_CLUSTER_API_URL: ${MANAGED_CLUSTER_API_URL}"

    oc login -u "$MANAGED_CLUSTER_USER" -p "$MANAGED_CLUSTER_PASS" "$MANAGED_CLUSTER_API_URL" # &>/dev/null

    # need to check if this is the correct CA file
    clusteradm join --hub-token ${HUB_TOKEN} --hub-apiserver ${HUB_API_URL} --cluster-name managed-${i} --ca-file /var/run/secrets/kubernetes.io/serviceaccount/ca.crt --wait # &>/dev/null
  fi
done
echo "Waiting for klusterlet registration agent..."
timeout 300 bash -c "until kubectl -n open-cluster-management-agent get pods | grep -q klusterlet-registration-agent.*Running; do sleep 10; done"

# Wait for klusterlet work agent to be ready
echo "Waiting for klusterlet work agent..."
timeout 300 bash -c "until kubectl -n open-cluster-management-agent get pods | grep -q klusterlet-work-agent.*Running; do sleep 10; done"
oc login -u "$HUB_USER" -p "$HUB_PASS" "$HUB_API_URL" # &>/dev/null
# Need to wait brefore accepting. Wait for each join to process and need timeout for each join.
clusteradm accept --clusters ${managed_clusters} --wait