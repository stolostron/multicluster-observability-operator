#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -exo pipefail

# creating the simulated managedcluster
for i in $(seq $1 $2); do
  echo "Creating Simulated managedCluster simulated-${i}-managedcluster..."
  cat <<EOF | kubectl apply -f -
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: simulated-${i}-managedcluster
spec:
  hubAcceptsClient: true
EOF
done
