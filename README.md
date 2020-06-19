# multicluster-monitoring-operator

## Overview

The multicluster-monitoring-operator is a component of ACM observability feature. It is designed to install into Hub Cluster.

<div align="center">
<img src="./docs/images/multicluster-monitoring-operator.png">
</div>

## Installation

We provided an easy way to install this operator into KinD cluster to verify some basic functionalities.

1. Clone this repo locally

```
git clone https://github.com/open-cluster-management/multicluster-monitoring-operator.git
```

2. Provide the username and password for downloading multicluster-monitoring-operator image from quay.io.

```
export DOCKER_USER=<quay.io username>
export DOCKER_PASS=<quay.io password>
```

3. Deploy using the ./tests/e2e/setup.sh script
```
./tests/e2e/setup.sh
```
If you want to install the latest multicluster-monitoring-operator image, you can find the latest tag here https://quay.io/repository/open-cluster-management/multicluster-monitoring-operator?tab=tags. Then install by
```
./tests/e2e/setup.sh quay.io/open-cluster-management/multicluster-monitoring-operator:<latest tag>
```

4. Access the grafana dashboard
- Option 1: Edit /etc/hosts to add 
```
127.0.0.1 grafana.local
```
Then access grafana dashboard by `http://grafana.local`
- Option 2: Forward the grafana port into local machine
```
kubectl port-forward -n open-cluster-management $(oc get pod -n open-cluster-management -lapp=grafana-test -o jsonpath='{.items[0].metadata.name}') 3001
```
Then access grafana dashboard by `http://127.0.0.1:3001`

## Developer Guide
The guide is used for developer to build and install the multicluster-monitoring-operator. It can be running in [kind][install_kind] if you don't have a OCP environment.

### Prerequisites

- [git][git_tool]
- [go][go_tool] version v1.13.9+.
- [docker][docker_tool] version 19.03+.
- [kubectl][kubectl_tool] version v1.14+.
- Access to a Kubernetes v1.11.3+ cluster.

### Install the Operator SDK CLI

Follow the steps in the [installation guide][install_guide] to learn how to install the Operator SDK CLI tool. It requires [version v0.17.0][operator_sdk_v0.17.0].
Or just use this command to download `operator-sdk` for Mac:
```
curl -L https://github.com/operator-framework/operator-sdk/releases/download/v0.17.0/operator-sdk-v0.17.0-x86_64-apple-darwin -o operator-sdk
```

### Build the Operator

- git clone this repository.
- `go mod vendor`
- `operator-sdk build <repo>/<component>:<tag>` for example: quay.io/multicluster-monitoring-operator:v0.1.0.
- Replace the image in `deploy/operator.yaml`.
- Update your namespace in `deploy/role_binding.yaml`
- Update your grafana.server.domain in `deploy/crds/monitoring.open-cluster-management.io_v1_multiclustermonitoring_cr.yaml`

### Deploy this Operator

1. If you are using aws environment, skip this step. the `StorageClass` is set as `gp2` by default. Prepare the `StorageClass` and `PersistentVolume` to apply into the existing environment. For example:
```
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
  name: standard
provisioner: kubernetes.io/no-provisioner
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: pv-volume-1
  labels:
    type: local
spec:
  storageClassName: standard
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Delete
  hostPath:
    path: "/mnt/thanos/teamcitydata1"
```
2. Customize the configuration for the operator (optional)
You can customize the operator by updating `deploy/crds/monitoring.open-cluster-management.io_v1_multiclustermonitoring_cr.yaml`. Below is a sample which has the configuration with default values. If you want to use customized value for one parameter, just need to specify that parameter in your own yaml file before deploy the operator.
```
apiVersion: monitoring.open-cluster-management.io/v1alpha1
kind: MultiClusterMonitoring
metadata:
  name: monitoring
spec:
  grafana:
    hostport: 3001
    replicas: 1
  imagePullPolicy: Always
  imagePullSecret: multiclusterhub-operator-pull-secret
  imageRepository: quay.io/open-cluster-management
  imageTagSuffix: ""
  objectStorageConfigSpec:
    config:
      access_key: minio
      bucket: thanos
      endpoint: minio:9000
      insecure: true
      secret_key: minio123
      storage: 1Gi
    type: minio
  observatorium:
    api:
      image: quay.io/observatorium/observatorium:master-2020-04-29-v0.1.1-14-gceac185
      version: master-2020-04-29-v0.1.1-14-gceac185
    apiQuery:
      image: quay.io/thanos/thanos:v0.12.0
      version: v0.12.0
    compact:
      image: quay.io/thanos/thanos:v0.12.0
      retentionResolution1h: 30d
      retentionResolution5m: 14d
      retentionResolutionRaw: 5d
      version: v0.12.0
    hashrings:
    - hashring: default
    objectStorageConfig:
      key: thanos.yaml
      name: thanos-objectstorage
    query:
      image: quay.io/thanos/thanos:v0.12.0
      version: v0.12.0
    queryCache:
      image: quay.io/cortexproject/cortex:master-fdcd992f
      replicas: 1
      version: master-fdcd992f
    receivers:
      image: quay.io/thanos/thanos:v0.12.0
      version: v0.12.0
    rule:
      image: quay.io/thanos/thanos:v0.12.0
      version: v0.12.0
    store:
      cache:
        exporterImage: prom/memcached-exporter:v0.6.0
        exporterVersion: v0.6.0
        image: docker.io/memcached:1.6.3-alpine
        memoryLimitMb: 1024
        replicas: 1
        version: 1.6.3-alpine
      image: quay.io/thanos/thanos:v0.12.0
      shards: 1
      version: v0.12.0
    thanosReceiveController:
      image: quay.io/observatorium/thanos-receive-controller:latest
      version: latest
  storageClass: gp2
  version: latest
```
3. Apply the manifests
```
kubectl apply -f deploy/req_crds/
kubectl apply -f deploy/crds/
kubectl apply -f deploy/

```
After installed successfully, you will see the following output:
`oc get pod`
```
NAME                                                              READY   STATUS    RESTARTS   AGE
grafana-deployment-846fd485fc-pmg6x                               1/1     Running   0          158m
grafana-operator-6fd7d76c6c-lzp6d                                 1/1     Running   0          158m
minio-5c8b47c889-vvfrz                                            1/1     Running   0          158m
monitoring-observatorium-cortex-query-frontend-5644474746-2tpsv   1/1     Running   0          158m
monitoring-observatorium-observatorium-api-gateway-6c4c475f4d5x   1/1     Running   0          158m
monitoring-observatorium-observatorium-api-gateway-thanos-vp2vm   1/1     Running   0          158m
monitoring-observatorium-thanos-compact-0                         1/1     Running   0          158m
monitoring-observatorium-thanos-query-698f99987f-xlndd            1/1     Running   0          158m
monitoring-observatorium-thanos-receive-controller-f5554fb9lnbj   1/1     Running   0          158m
monitoring-observatorium-thanos-receive-default-0                 1/1     Running   0          158m
monitoring-observatorium-thanos-receive-default-1                 1/1     Running   0          157m
monitoring-observatorium-thanos-receive-default-2                 1/1     Running   0          156m
monitoring-observatorium-thanos-rule-0                            1/1     Running   0          158m
monitoring-observatorium-thanos-rule-1                            1/1     Running   0          156m
monitoring-observatorium-thanos-store-shard-0-0                   1/1     Running   0          158m
multicluster-monitoring-operator-5d7fd6dffb-qgg6c                 1/1     Running   0          158m
observatorium-operator-84787d4b9c-28pd9                           1/1     Running   0          158m
```
`oc get grafana`
```
NAME                 AGE
monitoring-grafana   165m
```
`oc get observatorium`
```
NAME                       AGE
monitoring-observatorium   163m
```
### View metrics in dashboard
1. The Prometheus in hub cluster already enabled remoteWrite to send metrics. Access Grafana console at https://{YOUR_DOMAIN}/grafana, view the metrics in the dashboard named "ACM:Managed Cluster Monitoring"

2. Manually Enable remote write for OCP prometheus in managed clusters
Create the configmap in openshift-monitoring namespace. Replace the url with the your route value. Also need to replace the value of "replacement" with the spoke cluster's name
```
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-monitoring-config
  namespace: openshift-monitoring
data:
  config.yaml: |
    prometheusK8s:
      remoteWrite:
        - url: "http://observatorium-api-gateway-acm-monitoring.apps.one-chimp.dev05.red-chesterfield.com/api/metrics/v1/write"
          writeRelabelConfigs:
          - sourceLabels: [__name__]
            replacement: test_cluster
            targetLabel: cluster
```
The changes will be applied automatically after several minutes. You can apply the changes immediately by invoking command below
```
oc scale --replicas=2 statefulset --all -n openshift-monitoring; oc scale --replicas=1 deployment --all -n openshift-monitoring
```

### Endpoint monitoring operator installation & endpoint monitoring configuration
1. By default, the endpoint monitoring operator will be installed on any managed clusters. If want to disable this in a clusteer, need to add label using key/value "monitoring"/"disabled" on it.
2. Once the endpoint monitoring operator installed in the managed cluster, it will update the configmap cluster-monitoring-config automatically, and then the metrics will be pushed to hub side.
3. In cluster's namespace in hub side, one default endpointmonitoring resource named as "endpoint-config"  will be created automatically. Users can edit section "relabelConfigs" in this resource to update the configuration for metrics collect in managed cluster side, such as filtering the metrics collected, injecting addtional labels([Prometheus relabel configuration]). A sample endpointmonitoring resource is as below:
```
apiVersion: monitoring.open-cluster-management.io/v1alpha1
kind: EndpointMonitoring
metadata:
  annotations:
    owner: multicluster-operator
  name: endpoint-config
spec:
  global:
    serverUrl: observatorium-api-acm-monitoring.apps.marco.dev05.red-chesterfield.com
  metricsCollectors:
  - enable: true
    relabelConfigs:
    - replacement: spoke1
      sourceLabels:
      - __name__
      targetLabel: cluster
    type: OCP_PROMETHEUS
```

[install_kind]: https://github.com/kubernetes-sigs/kind
[install_guide]: https://github.com/operator-framework/operator-sdk/blob/master/doc/user/install-operator-sdk.md
[git_tool]:https://git-scm.com/downloads
[go_tool]:https://golang.org/dl/
[docker_tool]:https://docs.docker.com/install/
[kubectl_tool]:https://kubernetes.io/docs/tasks/tools/install-kubectl/
[operator_sdk_v0.17.0]:https://github.com/operator-framework/operator-sdk/releases/tag/v0.17.0
[Prometheus relabel configuration]:https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config
