# multicluster-observability-operator

## Overview

The multicluster-observability-operator is a component of ACM observability feature. It is designed to install into Hub Cluster.

## Installation

### Install this operator on RHACM

1. Clone this repo locally

```
git clone https://github.com/open-cluster-management/multicluster-monitoring-operator.git
git checkout origin/release-2.1
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
4. Create Secret for object storage

You need to create a Secret for object storage in advance. Firstly create the  object storage configuration should as following:

```
type: s3
config:
  bucket: YOUR-BUCKET
  endpoint: S3-ENDPOINT
  insecure: true
  access_key: ACCESS_KEY
  secret_key: SECRET_KEY
```

You will need to provide a value for the `bucket`, `endpoint`, `access_key`, and `secret_key` keys. For more details, please refer to https://thanos.io/tip/thanos/storage.md/#s3.

Then encode object storage configuration with base64, fill it in `example/object-storage-secret.yaml` `thanos.yaml`field, and create the Secret using folllowing commands:

Only for development or testing purposes, you can [deploy your object storage](./README.md#setup-object-storage).
```
$ cat your-object-storage-configuration | base64
$ oc apply --namespace=open-cluster-management-observability -f example/object-storage-secret.yaml
```

5. [Optional] Modify the operator and instance to use a new SNAPSHOT tag

Edit deploy/operator.yaml file and change image tag
```
    spec:
      serviceAccountName: multicluster-observability-operator
      containers:
        - name: multicluster-observability-operator
          # Replace this with the built image name
          image: ...

```

Edit observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml file to change `mco-imageTagSuffix` and `storageConfigObject`. In  `storageConfigObject.metricObjectStorage`, `name` is the name of the Secret created in  step 4, `key` is the key of the data in that Secret.

```
apiVersion: observability.open-cluster-management.io/v1alpha1
kind: MultiClusterObservability
metadata:
  name: observability
  annotations:
    mco-imageRepository: quay.io/open-cluster-management
    mco-imageTagSuffix: 2.1.0-SNAPSHOT-2020-08-11-14-16-48
spec:
  storageConfigObject:
    metricObjectStorage:
      name: thanos-object-storage
      key: thanos.yaml
```

Note: Find snapshot tags here: https://quay.io/repository/open-cluster-management/acm-custom-registry?tab=tags

6. [Optional] Customize the configuration for the operator instance

You can customize the operator instance by updating `observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml`. Below is a sample which has the configuration with default values. If you want to use customized value for one parameter, just need to specify that parameter in your own yaml file.

7. Deploy the `multicluster-observability-operator` to `open-cluster-management` namespace

```
oc project open-cluster-management
oc apply -f deploy/req_crds/observability.open-cluster-management.io_observabilityaddon_crd.yaml
oc apply -f deploy/req_crds/core.observatorium.io_observatoria.yaml
oc apply -f deploy/crds/observability.open-cluster-management.io_multiclusterobservabilities_crd.yaml
oc apply -f deploy/
```
When you successfully install the `multicluster-observability-operator`, the following pods are available in `open-cluster-management` namespace:

```
NAME                                                              READY   STATUS    RESTARTS   AGE
multicluster-observability-operator-55bc57d65c-tk2c2              1/1     Running   0          7h8m
```

8. Deploy `MultiClusterObservability` instance to `open-cluster-management-observability` namespace

```
oc project open-cluster-management-observability
oc apply -f deploy/crds/observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml
```

The following pods are available in `open-cluster-management-observability` namespace after installed successfully.

```
NAME                                                              READY   STATUS    RESTARTS   AGE
grafana-7cb7c6b698-4kbdc                                          1/1     Running   0          7h8m
observability-observatorium-cortex-query-frontend-56bd7954zk4hs   1/1     Running   0          7h6m
observability-observatorium-observatorium-api-7cbb7766b-k5lxf     1/1     Running   0          7h7m
observability-observatorium-thanos-compact-0                      1/1     Running   0          7h4m
observability-observatorium-thanos-query-6658db5979-5dvjq         1/1     Running   0          7h4m
observability-observatorium-thanos-receive-controller-5965wbpqs   1/1     Running   0          7h1m
observability-observatorium-thanos-receive-default-0              1/1     Running   0          7h1m
observability-observatorium-thanos-rule-0                         1/1     Running   0          7h
observability-observatorium-thanos-store-memcached-0              2/2     Running   0          7h5m
observability-observatorium-thanos-store-shard-0-0                1/1     Running   0          6h59m
observatorium-operator-686cc5bf6-l9zcx                            1/1     Running   0          7h8m
```

9. View metrics in dashboard

Access Grafana console at https://{YOUR_DOMAIN}/grafana, view the metrics in the dashboard named "ACM:Cluster Monitoring"

### Install this operator on KinD

We provided an easy way to install this operator into KinD cluster to verify some basic functionalities.

1. Clone this repo locally

```
git clone https://github.com/open-cluster-management/multicluster-monitoring-operator.git
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
If you want to install the latest multicluster-observability-operator image, you can find the latest tag here https://quay.io/repository/open-cluster-management/multicluster-monitoring-operator?tab=tags. Then install by
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
- `operator-sdk build <repo>/<component>:<tag>` for example: quay.io/multicluster-monitoring-operator:v0.1.0.
- Replace the image in `deploy/operator.yaml`.
- Update your namespace in `deploy/role_binding.yaml`

### Setup object storage

For development or testing purposes, you can set up your own object storage. We provide some examples for you to set up your own object storage through [Minio](https://min.io/), you can find these examples in `tests/e2e/minio/` path. You need to update the storageclass in `tests/e2e/minio/minio-pvc.yaml` firstly, then create minio deployment as follows:

```
$ oc create ns open-cluster-management-observability
namespace/open-cluster-management-observability created
$ oc apply -f tests/e2e/minio/
deployment.apps/minio created
persistentvolumeclaim/minio created
secret/thanos-object-storage created
service/minio created
```

When minio starts successfully, Edit `observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml` file to change `metricObjectStorage` field. Fill the Secret `name` and `data key` in `metricObjectStorage` field. The Secret should as following:

```
apiVersion: v1
data:
  thanos.yaml: dHlwZTogczMKY29uZmlnOgogIGJ1Y2tldDogInRoYW5vcyIKICBlbmRwb2ludDogIm1pbmlvOjkwMDAiCiAgaW5zZWN1cmU6IHRydWUKICBhY2Nlc3Nfa2V5OiAibWluaW8iCiAgc2VjcmV0X2tleTogIm1pbmlvMTIzIg==
kind: Secret
metadata:
  name: thanos-object-storage
type: Opaque
```

You can run the following command to get the object storage configuration:

```
$ kubectl get secret thanos-object-storage -o 'go-template={{index .data "thanos.yaml"}}' | base64 --decode
type: s3
config:
  bucket: "thanos"
  endpoint: "minio:9000"
  insecure: true
  access_key: "minio"
  secret_key: "minio123"
```

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
