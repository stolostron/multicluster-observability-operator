# multicluster-observability-operator

## Overview

The multicluster-observability-operator is a component of ACM observability feature. It is designed to install into Hub Cluster.

<div align="center">
<img src="./docs/images/multicluster-observability-operator.png">
</div>

## Installation

### Install this operator on RHACM

1. Clone this repo locally

```
git clone https://github.com/open-cluster-management/multicluster-observability-operator.git
```
2. Create new namespace `open-cluster-management-observability`

```
oc create namespace open-cluster-management-observability
```
3. Generate your pull-secret
Assume RHACM is installed in `open-cluster-management` namespace. Generate your pull-screct by

```
oc get secret multiclusterhub-operator-pull-secret -n open-cluster-management --export -o yaml |   kubectl apply --namespace=open-cluster-management-observability -f -
```
4. [Optional] Modify the operator and instance to use a new tag

Edit deploy/operator.yaml file and change image tag
```
    spec:
      serviceAccountName: multicluster-observability-operator
      containers:
        - name: multicluster-observability-operator
          # Replace this with the built image name
          image: ...

```
Edit deploy/crds/monitoring.open-cluster-management.io_v1alpha1_multiclustermonitoring_cr.yaml file to change `imageTagSuffix`
```
apiVersion: monitoring.open-cluster-management.io/v1alpha1
kind: MultiClusterObservability
metadata:
  name: monitoring
spec:
  imageTagSuffix: ...
```

Note: Find snapshot tags here: https://quay.io/repository/open-cluster-management/acm-custom-registry?tab=tags

5. [Optional] Customize the configuration for the operator instance
You can customize the operator instance by updating `deploy/crds/monitoring.open-cluster-management.io_v1_multiclustermonitoring_cr.yaml`. Below is a sample which has the configuration with default values. If you want to use customized value for one parameter, just need to specify that parameter in your own yaml file.
```
apiVersion: monitoring.open-cluster-management.io/v1alpha1
kind: MultiClusterObservability
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
      volumeClaimTemplate:
        spec:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 1Gi
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
      volumeClaimTemplate:
        spec:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 1Gi
    rule:
      image: quay.io/thanos/thanos:v0.12.0
      version: v0.12.0
      volumeClaimTemplate:
        spec:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 1Gi
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
      volumeClaimTemplate:
        spec:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 1Gi
    thanosReceiveController:
      image: quay.io/observatorium/thanos-receive-controller:master-2020-04-16-44c6bea
      version: master-2020-04-16-44c6bea
  storageClass: local
  version: latest
```

6. Deploy the `multicluster-observability-operator` and `MultiClusterObservability` instance
```
oc project open-cluster-management-observability
oc apply -f deploy/req_crds/monitoring.open-cluster-management.io_endpointmonitoring_crd.yaml
oc apply -f deploy/req_crds/core.observatorium.io_observatoria.yaml
oc apply -f deploy/crds/monitoring.open-cluster-management.io_multiclustermonitorings_crd.yaml
oc apply -f deploy/crds/monitoring.open-cluster-management.io_v1alpha1_multiclustermonitoring_cr.yaml
oc apply -f deploy/
```
The following pods are available in `open-cluster-management-observability` namespace after installed successfully.
```
NAME                                                                  READY   STATUS    RESTARTS   AGE
pod/grafana-6fb9584cf9-tt5s2                                          1/1     Running   0          76m
pod/minio-786cb78959-44mqj                                            1/1     Running   0          76m
pod/monitoring-observatorium-cortex-query-frontend-695bdfc9cd-c4tcm   1/1     Running   0          75m
pod/monitoring-observatorium-observatorium-api-6b474975f9-lgnfh       1/1     Running   0          76m
pod/monitoring-observatorium-observatorium-api-thanos-query-77kfwz7   1/1     Running   0          76m
pod/monitoring-observatorium-thanos-compact-0                         1/1     Running   0          74m
pod/monitoring-observatorium-thanos-query-85d8cd96d6-5jj74            1/1     Running   0          74m
pod/monitoring-observatorium-thanos-receive-controller-675868d4rzbq   1/1     Running   0          73m
pod/monitoring-observatorium-thanos-receive-default-0                 1/1     Running   0          73m
pod/monitoring-observatorium-thanos-receive-default-1                 1/1     Running   0          73m
pod/monitoring-observatorium-thanos-receive-default-2                 1/1     Running   0          73m
pod/monitoring-observatorium-thanos-rule-0                            1/1     Running   0          72m
pod/monitoring-observatorium-thanos-rule-1                            1/1     Running   0          72m
pod/monitoring-observatorium-thanos-store-memcached-0                 2/2     Running   0          75m
pod/monitoring-observatorium-thanos-store-shard-0-0                   1/1     Running   0          72m
pod/multicluster-observability-operator-5dc5997979-f4flc                 1/1     Running   1          77m
pod/observatorium-operator-88b859dc-79hml                             1/1     Running   0          76m
```

7. By default, the endpoint monitoring operator will be installed on any managed clusters to remote-write the metrics from managed clusters to hub cluster. [How to configure endpoint monitoring?](#endpoint-monitoring-operator-installation--endpoint-monitoring-configuration)

8. View metrics in dashboard
Access Grafana console at https://{YOUR_DOMAIN}/grafana, view the metrics in the dashboard named "ACM:Cluster Monitoring"

### Install this operator on KinD

We provided an easy way to install this operator into KinD cluster to verify some basic functionalities.

1. Clone this repo locally

```
git clone https://github.com/open-cluster-management/multicluster-observability-operator.git
```

2. Provide the username and password for downloading multicluster-observability-operator image from quay.io.

```
export DOCKER_USER=<quay.io username>
export DOCKER_PASS=<quay.io password>
```

3. Deploy using the ./tests/e2e/setup.sh script
```
./tests/e2e/setup.sh
```
If you want to install the latest multicluster-observability-operator image, you can find the latest tag here https://quay.io/repository/open-cluster-management/multicluster-observability-operator?tab=tags. Then install by
```
./tests/e2e/setup.sh quay.io/open-cluster-management/multicluster-observability-operator:<latest tag>
```

4. Access the KinD cluster

Access `hub` KinD cluster by `export KUBECONFIG=$HOME/.kube/kind-config-hub`
Access `hub` KinD cluster by `export KUBECONFIG=$HOME/.kube/kind-config-spoke`

## Developer Guide

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
- `operator-sdk build <repo>/<component>:<tag>` for example: quay.io/multicluster-observability-operator:v0.1.0.
- Replace the image in `deploy/operator.yaml`.
- Update your namespace in `deploy/role_binding.yaml`

### Endpoint monitoring operator installation & endpoint monitoring configuration
1. By default, the endpoint monitoring operator will be installed on any managed clusters. If want to disable this in a cluster, need to add label using key/value "monitoring"/"disabled" on it.
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
