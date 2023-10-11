#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -exo pipefail

KUBECTL="kubectl"
if ! command -v kubectl &>/dev/null; then
  if command -v oc &>/dev/null; then
    KUBECTL="oc"
  else
    echo "kubectl or oc must be installed!"
    exit 1
  fi
fi

# deleting the simulated managedcluster
for i in $(seq $1 $2); do
  echo "Deleting Simulated managedCluster simulated-${i}-managedcluster..."
  ${KUBECTL} delete managedcluster simulated-${i}-managedcluster
done
