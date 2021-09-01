# Metrics Collector Simulator

Metrics collector simulator can be used to setup multiple metrics collector in different namespaces in one managed cluster, to simulate thousands of managed clusters push metrics to ACM hub cluster for scale testing.

_Note:_ this simulator is for testing purpose only.

## Prereqs
You must meet the following requirements to setup metrics collector:

- ACM 2.1+ available
- `MultiClusterObservability` instance available and have following pods in `open-cluster-management-addon-observability` namespace:

	```
	$ oc get po -n open-cluster-management-addon-observability
	NAME                                               READY   STATUS    RESTARTS   AGE
	endpoint-observability-operator-7f8f949bc8-trwzh   2/2     Running   0          118m
	metrics-collector-deployment-74cbf5896f-jhg6v      1/1     Running   0          111m
	```

## Quick Start
### Setup metrics collector
You can run `setup-metrics-collector.sh` following with a number to setup multiple metrics collector.

For example, setup 10 metrics collectors with the following command:
```
# ./setup-metrics-collector.sh 10
```
Check if all the metrics collector running successfully in your cluster:
```
# oc get pods --all-namespaces | grep simulate-managed-cluster
simulate-managed-cluster1                          metrics-collector-deployment-7d69d9f897-xn8vz                    1/1     Running            0          22h
simulate-managed-cluster10                         metrics-collector-deployment-6698466fd8-9zxq2                    1/1     Running            0          22h
simulate-managed-cluster2                          metrics-collector-deployment-67844bfc59-lwchn                    1/1     Running            0          22h
simulate-managed-cluster3                          metrics-collector-deployment-56bc9485b4-gsxm9                    1/1     Running            0          22h
simulate-managed-cluster4                          metrics-collector-deployment-85d7dd974d-hcm6n                    1/1     Running            0          22h
simulate-managed-cluster5                          metrics-collector-deployment-76c9756648-pcw44                    1/1     Running            0          22h
simulate-managed-cluster6                          metrics-collector-deployment-7557ccb5c6-l7m44                    1/1     Running            0          22h
simulate-managed-cluster7                          metrics-collector-deployment-6994d95664-kb772                    1/1     Running            0          22h
simulate-managed-cluster8                          metrics-collector-deployment-6c8794b786-jm52h                    1/1     Running            0          22h
simulate-managed-cluster9                          metrics-collector-deployment-5fdcc96d99-gqwqf                    1/1     Running            0          22h
```

> Note: if you want the simulated metrics-collector be scheduled to master node, so that more simulated metrics-collectors can be deployed, you can set the environment variable `ALLOW_SCHEDULED_TO_MASTER` to be `true` before executing the setup script.

### Clean metrics collector
Use `clean-metrics-collector.sh` to remove all metrics collector you created.
```
# ./clean-metrics-collector.sh 10
```

## Generate your own metrics data source
By default, `setup-metrics-collector.sh` is using metrics data defined in env `METRICS_IMAGE` as data source. You can build and push your own metrics data image with below command:
```
# METRICS_IMAGE=<example/metrics-data:latest> make all
```
## Setup metrics collector with your own metrics data source
Running below command to setup metrics collectors with your own data source:
```
# METRICS_IMAGE=<example/metrics-data:latest> ./setup-metrics-collector.sh 10
```
