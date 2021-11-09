#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# default kube client is kubectl, use oc if kubectl is not installed
KUBECTL="kubectl"

if ! command -v kubectl &> /dev/null; then
    if command -v oc &> /dev/null; then
        KUBECTL="oc"
    else
        if [[ "$(uname)" == "Linux" ]]; then
            curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
        elif [[ "$(uname)" == "Darwin" ]]; then
            curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/darwin/amd64/kubectl"
        fi
        chmod +x ${PWD}/kubectl
        KUBECTL=${PWD}/kubectl
    fi
fi

SED_COMMAND='sed -e'
if [[ "$(uname)" == "Darwin" ]]; then
    SED_COMMAND='sed -e'
fi

# creating the simulated managedcluster
for i in $(seq $1 $2)
do
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
