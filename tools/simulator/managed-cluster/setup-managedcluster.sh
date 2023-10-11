#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -exo pipefail

WORK_DIR="$(
  cd "$(dirname "$0")"
  pwd -P
)"
# Create bin directory and add it to PATH
mkdir -p ${WORK_DIR}/bin
export PATH=${PATH}:${WORK_DIR}/bin

KUBECTL="kubectl"
if ! command -v kubectl &>/dev/null; then
  if command -v oc &>/dev/null; then
    KUBECTL="oc"
  else
    echo "This script will install kubectl (https://kubernetes.io/docs/tasks/tools/install-kubectl/) on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
      curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/linux/amd64/kubectl
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/darwin/amd64/kubectl
    fi
    chmod +x ./kubectl && mv ./kubectl ${WORK_DIR}/bin/kubectl
  fi
fi

# creating the simulated managedcluster
for i in $(seq $1 $2); do
  echo "Creating Simulated managedCluster simulated-${i}-managedcluster..."
  cat <<EOF | ${KUBECTL} apply -f -
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: simulated-${i}-managedcluster
spec:
  hubAcceptsClient: true
EOF
done
