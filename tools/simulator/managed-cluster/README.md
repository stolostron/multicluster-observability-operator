# Managed Cluster Simulator

The managed cluster simulator can be used to set up multiple managed clusters and create the corresponding namespaces 
in ACM hub cluster, to simulate reconciling thousands of managed clusters for the multicluster-observability-operator.

_Note:_ this simulator is for testing purpose only.

## Prerequisites

The following are requirements to set up the managed cluster simulator:

1. `ACM` version 2.3+
2. `MultiClusterObservability` instance available in the Hub cluster
3. `kubectl`

## How to use

### Set up managed cluster simulator

You can run `setup-managedcluster.sh` followed by two numbers (start and end index) to set up 
multiple simulated managed clusters. For example, set up five simulated `managedcluster` with the following command:

```bash
./setup-managedcluster.sh 1 5
```

Check if all the `managedcluster` are set up successfully in ACM hub cluster:

```bash
$ kubectl get managedcluster  | grep simulated
# simulated-1-managedcluster   true          46s
# simulated-2-managedcluster   true          46s
# simulated-3-managedcluster   true          45s
# simulated-4-managedcluster   true          44s
# simulated-5-managedcluster   true          44s
```

Check if the `Manifestwork` are created for the simulated managed clusters:

```bash
$ for i in $(seq 1 5); do kubectl -n simulated-$i-managedcluster get manifestwork --no-headers; done
# simulated-1-managedcluster-observability   72s
# simulated-2-managedcluster-observability   70s
# simulated-3-managedcluster-observability   69s
# simulated-4-managedcluster-observability   67s
# simulated-5-managedcluster-observability   65s
```

Clean up the simulated `managedclusters` by running the 
`clean-managedcluster.sh` script followed by two numbers (start and end index).

For example, clean up the previously created five simulated `managedclusters` with the following command:

```bash
$ ./clean-managedcluster.sh 1 5
# Deleting Simulated managedCluster simulated-1-managedcluster...
# managedcluster.cluster.open-cluster-management.io "simulated-1-managedcluster" deleted
# Deleting Simulated managedCluster simulated-2-managedcluster...
# managedcluster.cluster.open-cluster-management.io "simulated-2-managedcluster" deleted
# Deleting Simulated managedCluster simulated-3-managedcluster...
# managedcluster.cluster.open-cluster-management.io "simulated-3-managedcluster" deleted
# Deleting Simulated managedCluster simulated-4-managedcluster...
# managedcluster.cluster.open-cluster-management.io "simulated-4-managedcluster" deleted
# Deleting Simulated managedCluster simulated-5-managedcluster...
# managedcluster.cluster.open-cluster-management.io "simulated-5-managedcluster" deleted
```
