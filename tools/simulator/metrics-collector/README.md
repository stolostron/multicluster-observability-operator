# Metrics Collector Simulator

Metrics collector simulator can be used to setup multiple metrics collector in different namespaces in one managed cluster, to simulate thousands of managed clusters push metrics concurrenyly to ACM hub cluster for scale testing.

_Note:_ this simulator is for testing purpose only.

## Prereqs

You must meet the following requirements to setup metrics collector:

- ACM 2.1+ available
- `MultiClusterObservability` instance available and the following pods are running in `open-cluster-management-addon-observability` namespace:

	```
	$ oc get po -n open-cluster-management-addon-observability
	NAME                                               READY   STATUS    RESTARTS   AGE
	endpoint-observability-operator-7f8f949bc8-trwzh   2/2     Running   0          118m
	metrics-collector-deployment-74cbf5896f-jhg6v      1/1     Running   0          111m
	```

## How to use

### Setup metrics collector

1. Run `setup-metrics-collector.sh` script to setup multiple metrics collector, `-n` specifies the simulated metrics collector number, optional `-t` specifies the metrics data source type, can be "SNO"(default value) or "NON_SNO", and optional `-w` specifies the worker number for each simulated metrics collector, you can also specifies the simulated metrics collector name prefix by the `-m` flag. For example, setup 2 metrics collectors with 100 workers that collect the SNO metrics data by the following command:

```bash
# ./setup-metrics-collector.sh -n 2 -t SNO -w 100
```

2. Check if all the metrics collector running successfully in your cluster:

```bash
# oc get pods --all-namespaces | grep simulated-managed-cluster
simulate-managed-cluster1                          metrics-collector-deployment-7d69d9f897-xn8vz                    1/1     Running            0          22h
simulate-managed-cluster2                          metrics-collector-deployment-67844bfc59-lwchn                    1/1     Running            0          22h
```

> _Note:_ the above command will simulate 200 metrics collectors pushing the data concurrently into hub thanos.

> _Note:_ if you want the simulated metrics-collector be scheduled to master node, so that more simulated metrics-collectors can be deployed in one cluster, you can set the environment variable `ALLOW_SCHEDULED_TO_MASTER` to be `true` before executing the setup script.

### Clean metrics collector

Use `clean-metrics-collector.sh` to remove all the simulated metrics collector, `-n` specifies the simulated metrics collector number:

```bash
# ./clean-metrics-collector.sh -n 2
```

## Customize the metrics data source

### Generate your own data source

By default, `setup-metrics-collector.sh` is using metrics data defined in env `METRICS_IMAGE` as data source. You can build and push your own metrics data image with below command:

```bash
# METRICS_IMAGE=<example/metrics-data:latest> make all
```

## Setup metrics collector with your own metrics data source

Running below command to setup metrics collectors with your own data source:

```bash
# METRICS_IMAGE=<example/metrics-data:latest> ./setup-metrics-collector.sh -n 10
```
