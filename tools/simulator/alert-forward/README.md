# Alert Forward Simulator

The alert forward simulator can be used to simulate multiple Prometheus instances to forward alerts to the Alertmanager in the ACM hub cluster.

## Prereqs

You must meet the following requirements to setup metrics collector:

1. ACM 2.3+ available
2. `MultiClusterObservability` instance available in the hub cluster

## How to use

### Run locally outside the cluster

1. Export host of the Alertmanager in the ACM hub cluster:

```
export ALERTMANAGER_HOST=$(oc -n open-cluster-management-observability get route alertmanager -o jsonpath="{.spec.host}")
```

2. Export access token to the Alertmanager in the ACM hub cluster:

```
export ALERRTMANAGER_ACCESS_TOKEN=$(oc -n open-cluster-management-observability get secret $(oc -n open-cluster-management-observability get sa observability-alertmanager-accessor -o yaml | grep observability-alertmanager-accessor-token | cut -d' ' -f3) -o jsonpath="{.data.token}" | base64 -d)
```

3. Run the simulator to send fake alerts to the Alertmanager in ACM hub cluster:
```
go run main.go --am-host=${ALERTMANAGER_HOST} --am-access-token=${ALERRTMANAGER_ACCESS_TOKEN} --alerts-file=tools/simulator/alert-forward/alerts.json
```

> Note: you can also optionally specify the number of concurrent goroutines that forward the alerts by `--workers` flag and the alert forward interval by `--interval` flag.

### Run as a Deployment inside the cluster

1. (Optional) Build and push the alert-forwarder image:
```
docker build -f Dockerfile -t quay.io/ocm-observability/alert-forwarder:2.4.0 ../../..
docker push quay.io/ocm-observability/alert-forwarder:2.4.0
```

2. Run the following command to deploy the alert-forwarder:

```
./setup-alert-forwarder.sh
```

3. Check if all the alert-forwarder pod is running successfully in your cluster:

```
# oc -n alert-forwarder get pod
NAME                              READY   STATUS    RESTARTS   AGE
alert-forwarder-fb75bbb8c-6zgq8   1/1     Running   0          3m11s
```

