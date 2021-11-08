# Managed Cluster Simulator

The managed cluster simulator can be used to set up multiple managed clusters and create the corresponding namespaces in ACM hub cluster, to simulate reconciling thousands of managed clusters for the multicluster-observability-operator.

_Note:_ this simulator is for testing purpose only.

## Prereqs

You must meet the following requirements to setup managed cluster simulator:

1. ACM 2.3+ available
2. `MultiClusterObservability` instance available in the hub cluster

## How to use

### Set up managed cluster simulator

1. You can run `setup-managedcluster.sh` followed with two numbers(start index and end index) to set up multiple simulated managed clusters. For example, set up 1-10 simulated managedcluster with the following command:

```bash
# ./setup-managedcluster.sh 1 5
Creating Simulated managedCluster simulated-1-managedcluster...
managedcluster.cluster.open-cluster-management.io/simulated-1-managedcluster created
Creating Simulated managedCluster simulated-2-managedcluster...
managedcluster.cluster.open-cluster-management.io/simulated-2-managedcluster created
Creating Simulated managedCluster simulated-3-managedcluster...
managedcluster.cluster.open-cluster-management.io/simulated-3-managedcluster created
Creating Simulated managedCluster simulated-4-managedcluster...
managedcluster.cluster.open-cluster-management.io/simulated-4-managedcluster created
Creating Simulated managedCluster simulated-5-managedcluster...
managedcluster.cluster.open-cluster-management.io/simulated-5-managedcluster created
```

2. Check if all the managed cluster are set up successfully in ACM hub cluster:

```bash
$ oc get managedcluster  | grep simulated
simulated-1-managedcluster   true                                                                                       46s
simulated-2-managedcluster   true                                                                                       46s
simulated-3-managedcluster   true                                                                                       45s
simulated-4-managedcluster   true                                                                                       44s
simulated-5-managedcluster   true                                                                                       44s
```

3. Check if the `Manifestwork` are created for the simulated managed clusters:

```bash
$ for i in $(seq 1 5); do oc -n simulated-$i-managedcluster get manifestwork --no-headers; done
simulated-1-managedcluster-observability   72s
simulated-2-managedcluster-observability   70s
simulated-3-managedcluster-observability   69s
simulated-4-managedcluster-observability   67s
simulated-5-managedcluster-observability   65s
```

4. Clean up the simulated managed clusters by running the `clean-managedcluster.sh` script followed with two numbers(start index and end index), For example, clean up 1-10 simulated managedcluster with the following command:

```
$ ./clean-managedcluster.sh 1 5
Deleting Simulated managedCluster simulated-1-managedcluster...
managedcluster.cluster.open-cluster-management.io "simulated-1-managedcluster" deleted
Deleting Simulated managedCluster simulated-2-managedcluster...
managedcluster.cluster.open-cluster-management.io "simulated-2-managedcluster" deleted
Deleting Simulated managedCluster simulated-3-managedcluster...
managedcluster.cluster.open-cluster-management.io "simulated-3-managedcluster" deleted
Deleting Simulated managedCluster simulated-4-managedcluster...
managedcluster.cluster.open-cluster-management.io "simulated-4-managedcluster" deleted
Deleting Simulated managedCluster simulated-5-managedcluster...
managedcluster.cluster.open-cluster-management.io "simulated-5-managedcluster" deleted
```

