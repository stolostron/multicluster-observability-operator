#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# default kube client is kubectl, use oc if kubectl is nit installed
KUBECLIENT="kubectl"

if ! command -v kubectl &> /dev/null; then
    if command -v oc &> /dev/null; then
        KUBECLIENT="oc"
    else
        if [[ "$(uname)" == "Linux" ]]; then
            curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
        elif [[ "$(uname)" == "Darwin" ]]; then
            curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/darwin/amd64/kubectl"
        fi
        chmod +x ${PWD}/kubectl
        KUBECLIENT=${PWD}/kubectl
    fi
fi

SED_COMMAND='sed -e'
if [[ "$(uname)" == "Darwin" ]]; then
    SED_COMMAND='sed -e'
fi

# temporal working directory
WORKDIR=$(mktemp -d)
${KUBECLIENT} get managedcluster local-cluster -o yaml > ${WORKDIR}/simulated-managedcluster.yaml

# creating the simulated managedcluster
for index in $(seq $1 $2)
do
    echo "Creating Simulated managedCluster simulated-${index}-managedcluster..."
    ${KUBECLIENT} create ns simulated-${index}-managedcluster --dry-run -o yaml | ${KUBECLIENT} apply -f -
	${SED_COMMAND} "s~local-cluster~simulated-${index}-managedcluster~" ${WORKDIR}/simulated-managedcluster.yaml | ${KUBECLIENT} apply -f -
done

