# Metrics Collector Simulator

Metrics collector simulator can be used to setup multiple metrics collectors inside or outside of the cluster, to simulate thousands of managed clusters pushing metrics concurrently to ACM hub cluster for scalability testing.

_Note:_ this simulator is for testing purpose only.

## Prereqs

You must meet the following requirements to setup metrics collector:

- ACM 2.1+ available
- `MultiClusterObservability` instance available and `metrics-collector` pod is running in `open-cluster-management-addon-observability` namespace:

```bash
$ oc get pod -n open-cluster-management-addon-observability -l component=metrics-collector
NAME                                           READY   STATUS    RESTARTS   AGE
metrics-collector-deployment-695c5fbd8-l2m89   1/1     Running   0          5m
```

## How to use

### Run locally outside the cluster

1. Get the host of the metrics remote write address in ACM hub cluster:

```bash
export TO_UPLOAD_HOST=$(oc -n open-cluster-management-observability get route observatorium-api -o jsonpath="{.spec.host}")
```

2. Retrieve the CA certificate used to verify the metrics remote write address:

```bash
oc -n open-cluster-management-addon-observability get secret observability-managed-cluster-certs -o jsonpath="{.data.ca\.crt}" | base64 -d > ca.crt
```

3. Get the certificate and private key used to secure the request to the metrics remote write address:

```bash
oc -n open-cluster-management-addon-observability get secret observability-controller-open-cluster-management.io-observability-signer-client-cert -o jsonpath="{.data.tls\.crt}" | base64 -d > tls.crt
oc -n open-cluster-management-addon-observability get secret observability-controller-open-cluster-management.io-observability-signer-client-cert -o jsonpath="{.data.tls\.key}" | base64 -d > tls.key
```

4. Set the name and ID of simulated managed cluster, for example:

```bash
export SIMULATED_MANAGED_CLUSTER_NAME=simulated-sno-1
export SIMULATED_MANAGED_CLUSTER_ID=2b4bfc20-110e-4c4e-aa42-d97ac608c5e8
```

5. Retrieve the simulated metrics by following the instructions in [README here](metrics-extractor/README.md) .
 The script used for this earlier - `generate-metrics-data.sh` - is deprecated.

6. Run the metrics-collector to remotely write simulated SNO metrics to the ACM hub by running the following command:

```bash
$ export STANDALONE=true && go run ../../../collectors/metrics/cmd/metrics-collector/main.go \
	--to-upload https://${TO_UPLOAD_HOST}/api/metrics/v1/default/api/v1/receive \
	--to-upload-ca ./ca.crt \
	--to-upload-cert ./tls.crt \
	--to-upload-key ./tls.key \
	--simulated-timeseries-file=./timeseries.txt \
	--label="cluster=${SIMULATED_MANAGED_CLUSTER_NAME}" \
	--label="clusterID=${SIMULATED_MANAGED_CLUSTER_ID}"
level=info caller=logger.go:45 ts=2021-11-19T07:58:39.011221342Z msg="metrics collector initialized"
...
level=debug caller=logger.go:40 ts=2021-11-19T07:58:39.117297417Z component=forwarder component=metricsclient timeseriesnumber=3667
level=debug caller=logger.go:40 ts=2021-11-19T07:58:39.122690473Z component=forwarder component=metricsclient timeseriesnumber=3667
level=info caller=logger.go:45 ts=2021-11-19T07:58:39.250981391Z component=forwarder component=metricsclient msg="Metrics pushed successfully"
level=info caller=logger.go:45 ts=2021-11-19T07:58:39.267185279Z component=forwarder component=metricsclient msg="Metrics pushed successfully"
```

7. Optionally specify the number of concurrent workers that push the metrics by `--worker-number` flag, the default value is `1`.

8. Optionally specify the interval of pushing the metrics by `--interval` flag, the default value is `300s`.

### Run as a Deployment inside the cluster

1. Run `setup-metrics-collector.sh` script to setup multiple metrics collector, `-n` specifies the simulated metrics collector number, optional `-t` specifies the metrics data source type, can be "SNO"(default value) or "NON_SNO", and optional `-w` specifies the worker number for each simulated metrics collector, you can also specifies the simulated metrics collector name prefix by the `-m` flag. For example, setup 2 metrics collectors with 100 workers that collect the SNO metrics data by the following command:

```bash
./setup-metrics-collector.sh -n 2 -t SNO -w 100
```

2. Check if all the metrics collector running successfully in your cluster:

```bash
$ oc get pods --all-namespaces | grep simulated-managed-cluster
simulate-managed-cluster1                          metrics-collector-deployment-7d69d9f897-xn8vz                    1/1     Running            0          22h
simulate-managed-cluster2                          metrics-collector-deployment-67844bfc59-lwchn                    1/1     Running            0          22h
```

> _Note:_ the above command will simulate 200 metrics collectors pushing the data concurrently into hub thanos.

### Clean metrics collector

Use `clean-metrics-collector.sh` to remove all the simulated metrics collector, `-n` specifies the simulated metrics collector number:

```bash
./clean-metrics-collector.sh -n 2
```

## Customize the metrics data source

### Generate your own data source

By default, `setup-metrics-collector.sh` is using metrics data defined in env `METRICS_IMAGE` as data source. If you want to build and publish your own metrics data image, you must log into an OCP cluster and then execute the following command:

```bash
METRICS_IMAGE=<example/metrics-data:latest> ./generate-metrics-data.sh
```

## Setup metrics collector with your own metrics data source

Running below command to setup metrics collectors with your own data source:

```bash
METRICS_IMAGE=<example/metrics-data:latest> ./setup-metrics-collector.sh -n 10
```
