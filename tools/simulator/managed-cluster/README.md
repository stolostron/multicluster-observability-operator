# Managed Cluster Simulator

The managed cluster simulator can be used to set up multiple managed clusters and create the corresponding namespaces in the ACM hub cluster, to simulate reconciling thousands of managed clusters for the multicluster-observability-operator.

## Prereqs

You must meet the following requirements to setup metrics collector:

1. ACM 2.1+ available
2. `MultiClusterObservability` instance available in the hub cluster

## Quick Start

### Scale down the controllers

Before creating simulated managed clusters, we should scale down cluster-manager and controllers for managedcluster and manifestwork, to avoid resource conflict with the multicluster-observability-operator. Execute the following command:

```bash
kubectl -n open-cluster-management scale deploy cluster-manager --replicas 0
kubectl -n open-cluster-management-hub scale deploy cluster-manager-registration-controller --replicas 0
kubectl -n open-cluster-management-agent scale deploy klusterlet --replicas 0
kubectl -n open-cluster-management-agent scale deploy klusterlet-registration-agent --replicas 0
kubectl -n open-cluster-management-agent scale deploy klusterlet-work-agent --replicas 0
```

> Note: to make sure the controllers are not scaled up again by the operator and OLM, we also need to edit the CSV in the `open-cluster-management` to update the replicas of `cluster-manager` to be `0`.

### Set up managed cluster simulator

You can run `setup-managedcluster.sh` following with two numbers(start index and end index) to set up multiple simulated managedcluster.

For example, set up 1-10 simulated managedcluster with the following command:

```bash
# ./setup-managedcluster.sh 1 10
```

Check if all the metrics collector running successfully in your cluster:

```bash
# kubectl get managedcluster
NAME                           HUB ACCEPTED   MANAGED CLUSTER URLS                                                   JOINED   AVAILABLE   AGE
local-cluster                  true           https://api.obs-china-aws-4616-smzbp.dev05.red-chesterfield.com:6443   True     True        2d2h
simulated-1-managedcluster     true           https://api.obs-china-aws-4616-smzbp.dev05.red-chesterfield.com:6443            Unknown     1m
simulated-2-managedcluster     true           https://api.obs-china-aws-4616-smzbp.dev05.red-chesterfield.com:6443            Unknown     1m
simulated-3-managedcluster     true           https://api.obs-china-aws-4616-smzbp.dev05.red-chesterfield.com:6443            Unknown     1m
simulated-4-managedcluster     true           https://api.obs-china-aws-4616-smzbp.dev05.red-chesterfield.com:6443            Unknown     1m
simulated-5-managedcluster     true           https://api.obs-china-aws-4616-smzbp.dev05.red-chesterfield.com:6443            Unknown     1m
simulated-6-managedcluster     true           https://api.obs-china-aws-4616-smzbp.dev05.red-chesterfield.com:6443            Unknown     1m
simulated-7-managedcluster     true           https://api.obs-china-aws-4616-smzbp.dev05.red-chesterfield.com:6443            Unknown     1m
simulated-8-managedcluster     true           https://api.obs-china-aws-4616-smzbp.dev05.red-chesterfield.com:6443            Unknown     1m
simulated-9-managedcluster     true           https://api.obs-china-aws-4616-smzbp.dev05.red-chesterfield.com:6443            Unknown     1m
simulated-10-managedcluster    true           https://api.obs-china-aws-4616-smzbp.dev05.red-chesterfield.com:6443            Unknown     1m
```

