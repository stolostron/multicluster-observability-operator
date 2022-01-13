# multicluster-observability-operator

## Overview

The `multicluster-observability-operator` is a component for Red Hat Advanced Cluster Management for Kubernetes observability feature. `multicluster-observability-operator` is installed automatically in the hub cluster.

**Prerequisites**:

* You must install a Red Hat Advanced Cluster Management hub cluster.
* You must create a secret for object storage. For example, you can use Thanos as a storage solution. For more information, see [Thanos documentation](https://thanos.io/tip/thanos/storage.md/#configuration).

## Installation

Install the `multicluster-observability-operator` on Red Hat Advanced Cluster Management to visualize and monitor the health of your managed clusters. Complete the following steps:

1. Log in to your OpenShift Container Platform cluster by running the following command:

   ```
   oc login --token=YOUR_TOKEN --server=YOUR_OCP_CLUSTER
   ```

2. Create a new namespace for the observability service. For example, run the following command to create `open-cluster-management-observability`namespace:

   ```
   oc create namespace open-cluster-management-observability
   ```

3. Generate your pull-secret. If Red Hat Advanced Cluster Management is installed in the `open-cluster-management` namespace, run the following command to generate your secret:

   ```
   DOCKER_CONFIG_JSON=`oc extract secret/multiclusterhub-operator-pull-secret -n open-cluster-management --to=-`
   oc create secret generic multiclusterhub-operator-pull-secret \
       -n open-cluster-management-observability \
       --from-literal=.dockerconfigjson="$DOCKER_CONFIG_JSON" \
       --type=kubernetes.io/dockerconfigjson
   ```

   If the `multiclusterhub-operator-pull-secret` is not defined in the `open-cluster-management` namespace, copy the `pull-secret` from the `openshift-config` namespace into the `open-cluster-management-observability` namespace. Run the following command:

   ```
   DOCKER_CONFIG_JSON=`oc extract secret/pull-secret -n openshift-config --to=-`
   oc create secret generic multiclusterhub-operator-pull-secret \
       -n open-cluster-management-observability \
       --from-literal=.dockerconfigjson="$DOCKER_CONFIG_JSON" \
       --type=kubernetes.io/dockerconfigjson
   ```

4. Create and save the secret for object storage. For example, create a secret with Thanos on a Amazon Web Service cluster. Your file might resemble the following information:

   ```
   apiVersion: v1
   stringData:
     thanos.yaml: |
       type: s3
       config:
         bucket: YOUR_S3_BUCKET
         endpoint: YOUR_S3_ENDPOINT
         insecure: false
         access_key: YOUR_ACCESS_KEY
         secret_key: YOUR_SECRET_KEY
   metadata:
     name: thanos-object-storage
   type: Opaque
   kind: Secret
   ```
    You will need to provide a value for the bucket, endpoint, access_key, and secret_key keys above. For more details, please refer to https://thanos.io/tip/thanos/storage.md/#s3.

5. Apply the `object-storage-secret.yaml` by running the folllowing command:

   ```
   oc apply --namespace=open-cluster-management-observability -f object-storage-secret.yaml
   ```

   For development or testing purposes, you can [deploy your object storage](./README.md#setup-object-storage).

6. Deploy the observability feature by running the following command:

   ```
   oc apply -f deploy/crds/observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml
   ```

   You should be able to have the following pods in `open-cluster-management-observability` namespace.

   ```
   NAME                                                              READY   STATUS    RESTARTS   AGE
   alertmanager-0                                                    2/2     Running   0          8m57s
   alertmanager-1                                                    2/2     Running   0          8m36s
   alertmanager-2                                                    2/2     Running   0          8m18s
   grafana-658d56d649-6p9q5                                          2/2     Running   0          8m59s
   grafana-658d56d649-vpcth                                          2/2     Running   0          8m59s
   observability-observatorium-observatorium-api-6884555dd7-7zzff    1/1     Running   0          8m36s
   observability-observatorium-observatorium-api-6884555dd7-hfp6g    1/1     Running   0          8m36s
   observability-observatorium-thanos-compact-0                      1/1     Running   0          8m36s
   observability-observatorium-thanos-compact-1                      1/1     Running   0          8m21s
   observability-observatorium-thanos-compact-2                      1/1     Running   0          8m2s
   observability-observatorium-thanos-query-55c767cc97-64njg         1/1     Running   0          8m36s
   observability-observatorium-thanos-query-55c767cc97-t5nmw         1/1     Running   0          8m36s
   observability-observatorium-thanos-query-frontend-578d44f97c8tf   1/1     Running   0          8m36s
   observability-observatorium-thanos-query-frontend-578d44f9jq647   1/1     Running   0          8m36s
   observability-observatorium-thanos-receive-controller-5965wd5vk   1/1     Running   0          8m35s
   observability-observatorium-thanos-receive-default-0              1/1     Running   0          8m36s
   observability-observatorium-thanos-receive-default-1              1/1     Running   0          8m9s
   observability-observatorium-thanos-receive-default-2              1/1     Running   0          7m38s
   observability-observatorium-thanos-rule-0                         1/1     Running   0          8m36s
   observability-observatorium-thanos-rule-1                         1/1     Running   0          8m10s
   observability-observatorium-thanos-rule-2                         1/1     Running   0          7m39s
   observability-observatorium-thanos-store-memcached-0              2/2     Running   0          8m36s
   observability-observatorium-thanos-store-memcached-1              2/2     Running   0          8m32s
   observability-observatorium-thanos-store-memcached-2              2/2     Running   0          8m29s
   observability-observatorium-thanos-store-shard-0-0                1/1     Running   0          8m36s
   observability-observatorium-thanos-store-shard-1-0                1/1     Running   0          8m36s
   observability-observatorium-thanos-store-shard-2-0                1/1     Running   0          8m36s
   observatorium-operator-6c6d88c5db-x7xss                           1/1     Running   0          8m58s
   rbac-query-proxy-7ffb8bc6c9-pt96z                                 1/1     Running   0          8m58s
   rbac-query-proxy-7ffb8bc6c9-z7sng                                 1/1     Running   0          8m58s
   ```

7. View metrics in Grafana by navigating to the following URL: https://{YOUR_ACM_CONSOLE_DOMAIN}/grafana. The metrics are in the dashboard named _ACM:Cluster Monitoring_.

8. [Optional] You can delete the MulticlusterObservability resource by running the following command:

   ```
   oc delete mco observability #Your customized name of MulticlusterObservability CR
   ```

9. [Optional] You can scale the MultiClusterObservability deployment to zero to cease data collection.

## Customizing _multicluster-observability-operator_

You can customize the operand by updating `observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml`. View the following `multicluster-observability-operator` file with default values:

```
apiVersion: observability.open-cluster-management.io/v1beta1
kind: MultiClusterObservability
metadata:
  name: observability #Your customized name of MulticlusterObservability CR
spec:
  availabilityConfig: High # Available values are High or Basic
  imagePullPolicy: Always
  imagePullSecret: multiclusterhub-operator-pull-secret
  observabilityAddonSpec: # The ObservabilityAddonSpec defines the global settings for all managed clusters which have observability add-on enabled
    enableMetrics: true # EnableMetrics indicates the observability addon push metrics to hub server
    interval: 30 # Interval for the observability addon push metrics to hub server
  retentionResolution1h: 30d # How long to retain samples of 1 hour in bucket
  retentionResolution5m: 14d
  retentionResolutionRaw: 5d
  storageConfigObject: # Specifies the storage to be used by Observability
    metricObjectStorage:
      name: thanos-object-storage
      key: thanos.yaml
    statefulSetSize: 10Gi # The amount of storage applied to the Observability stateful sets, i.e. Thanos store, rule, compact and receiver.
    statefulSetStorageClass: gp2
```

   For `statefulSetStorageClass` field, view the following scenarios of the operator:

   - If you specify that a storage class does not exist, the operator uses the default storage class.
   - If you specify that a storage class exists and no previous PersistentVolumeClaim (PVC), the operator uses the specified storage class
   - If you specify that a storage class exists and use the previous PVC, the operator uses the PVC directly.

## Developer Guide

**Prerequisites**:

- Install [Git](https://git-scm.com/downloads).
- Install [Go](https://golang.org/dl/) version v1.13.9+.
- Install [Docker](https://docs.docker.com/install/) version 19.03+.
- Install [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) version v1.14+.
- Access to a Kubernetes v1.11.3+ cluster.
- Install [Operator_SDK CLI_tool v0.17.0](https://github.com/operator-framework/operator-sdk/releases/tag/v0.17.0).
- Install [Prometheus relabel configuration](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config).

### Install the Operator SDK CLI

Run the following command to install the Operator SDK CLI tool:

```
curl -L https://github.com/operator-framework/operator-sdk/releases/download/v0.17.0/operator-sdk-v0.17.0-x86_64-apple-darwin -o operator-sdk
```

### Build the observability operator

Complete the following steps to build the observability operator:

1. Clone the `open-cluster-management/multicluster-monitoring-operator` repository locally. Run the following command:

   ```
   git clone https://github.com/stolostron/multicluster-monitoring-operator.git
   ```

2. Run the following command to access your vendor:

  ```
  go mod vendor
  ```

3. Access the Operator SDK repository from Quay. For example, your URL might resemble the following: quay.io/multicluster-monitoring-operator:v0.1.0.

4. Replace the vaule for `image` in the `deploy/operator.yaml` file with the image that you built.

5. Update your namespace in the `deploy/role_binding.yaml` file.

### Setup object storage

For development or testing purposes, you can set up your own object storage. We provide some examples for you to set up your own object storage through [Minio](https://min.io/), you can find these examples in `tests/e2e/minio/` path. You need to update the storageclass in `tests/e2e/minio/minio-pvc.yaml` first, then create a Minio deployment.

Run the following commands to setup the object storage:

```
oc create ns open-cluster-management-observability
namespace/open-cluster-management-observability created


oc apply -f tests/e2e/minio/
deployment.apps/minio created
persistentvolumeclaim/minio created
secret/thanos-object-storage created
service/minio created
```

When Minio starts successfully, edit the `observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml` file to change the `metricObjectStorage` field. Fill the secret `name` and `data key` in `metricObjectStorage` field. Your Secret resource might resemble the following information:

```
apiVersion: v1
data:
  thanos.yaml: dHlwZTogczMKY29uZmlnOgogIGJ1Y2tldDogInRoYW5vcyIKICBlbmRwb2ludDogIm1pbmlvOjkwMDAiCiAgaW5zZWN1cmU6IHRydWUKICBhY2Nlc3Nfa2V5OiAibWluaW8iCiAgc2VjcmV0X2tleTogIm1pbmlvMTIzIg==
kind: Secret
metadata:
  name: thanos-object-storage
type: Opaque
```

You can access object storage configuration by running the following command:

```
kubectl get secret thanos-object-storage -o 'go-template={{index .data "thanos.yaml"}}' | base64 --decode
```
```
type: s3
config:
  bucket: "thanos"
  endpoint: "minio:9000"
  insecure: true
  access_key: "minio"
  secret_key: "minio123"
```

### Endpoint monitoring operator installation & endpoint monitoring configuration

1. By default, the endpoint monitoring operator is installed on any managed clusters. If you want to disable this in a cluster, you must update the configuration file using key/value "observability"/"disabled" on it.

2. Once the endpoint monitoring operator installed in the managed cluster, the `multicluster-monitoring-config` updates automatically. Metrics are pushed to your hub cluster.

3. The `multicluster-endpoint-config` is automatically create in the hub cluster namespace. Update the `multicluster-endpoint-config` to update the configuration  for metrics collection on your managed cluster. You can also add labels.

Update the labels in the EndpointMonitoring file. Your `endpointmonitoring` file might resemble the following contents:

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
