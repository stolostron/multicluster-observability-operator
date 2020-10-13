# Setup metrics collector

You can use `setup-metrics-collector.sh` to setup metrics collector to simulate multiple clients push metric data to ACM Hub. This script is for testing purposes only.

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

## Setup metrics collector

Use `setup-metrics-collector.sh` to setup metrics collector, you just need provide a number, then this script will create metrics collector in a different namespace.

```
./setup-metrics-collector.sh 10
```

## Clean metrics collector

Use `clean-metrics-collector.sh` to remove all metrics collector you created.

```
./clean-metrics-collector.sh 10
```