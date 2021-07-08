
# endpoint-monitoring-operator

## Overview

The endpoint-monitoring-operator is a component of ACM observability feature. It is designed to install into Spoke Cluster.


## Developer Guide
The guide is used for developer to build and install the endpoint-monitoring-operator . It can be running in [kind][install_kind] if you don't have a OCP environment.

### Prerequisites

- git
- go version v1.15+
- docker version 17.03+
- kubectl version v1.16.3+
- kustomize version v3.8.5+
- operator-sdk version v1.4.2+
- access to a Kubernetes v1.16.0+ cluster

### Build the Operator

1. Check out the endpoint-metrics-operator repository.

```
$ git clone git@github.com:open-cluster-management/endpoint-metrics-operator.git
```

2. Build the endpoint-metrics-operator image and push it to a public registry, such as quay.io:

```
$ make -f Makefile.prow docker-build docker-push IMG=quay.io/<YOUR_USERNAME_IN_QUAY>/endpoint-metrics-operator:latest
```

### Deploy this Operator

1. Create the `open-cluster-management-addon-observability` namespace if it doesn't exist:

```
$ kubectl create ns open-cluster-management-addon-observability
```

2. Create the secret named `hub-kube-config` in namespace `open-cluster-management-addon-observability` about the hub cluster information:

```
$ cat << EOF | kubectl apply -n open-cluster-management-addon-observability -f -
kind: Secret
apiVersion: v1
metadata:
  name: hub-kube-config
type: Opaque
data:
    kubeconfig: ***
EOF
```

> Note: the content of `kubeconfig` is base64-encoded content of kubeconfig for the hub cluster.

3. Create the secret named `hub-info-secret` in namespace `open-cluster-management-addon-observability` about the hub cluster information:

```
$ cat << EOF | kubectl apply -n open-cluster-management-addon-observability -f -
kind: Secret
apiVersion: v1
metadata:
  name: hub-info-secret
type: Opaque
data:
    clusterName: ***
    hub-info.yaml: ***
EOF
```

> Note: the content of `clusterName` is base64-encoded yaml of the hub cluster name, while the content of `hub-info.yaml` is base64-encoded yaml that contains the observatorium api gateway URL, hub alertmanager URl and hub router CA which is exposed on the hub cluster. The original yaml content resembles below:

```yaml
endpoint: "http://observatorium-api-open-cluster-management-observability.apps.stage3.demo.red-chesterfield.com/api/v1/receive"
hub-alertmanager-endpoint: "https://alertmanager-open-cluster-management-observability.apps.stage3.demo.red-chesterfield.com"
hub-alertmanager-router-ca: |
-----BEGIN CERTIFICATE-----
xxxxxxxxxxxxxxxxxxxxxxxxxxx
-----END CERTIFICATE-----
```

4. Create the configmap named `observability-metrics-allowlist` in namespace `open-cluster-management-addon-observability`:

```
$ kubectl apply -n open-cluster-management-addon-observability -f https://raw.githubusercontent.com/open-cluster-management/multicluster-observability-operator/main/manifests/base/config/metrics_allowlist.yaml
```

5. Update the value of environment variable `COLLECTOR_IMAGE` in the endpoint-metrics-operator deployment, for example: `quay.io/open-cluster-management/metrics-collector:2.3.0-SNAPSHOT-2021-04-08-09-07-10`

```
$ sed -i 's~REPLACE_WITH_METRICS_COLLECTOR_IMAGE~quay.io/open-cluster-management/metrics-collector:2.3.0-SNAPSHOT-2021-04-08-09-07-10~g' config/manager/manager.yaml
```

6. Update the value of environment variable `HUB_NAMESPACE` with the actual hub namespace, for example: `cluster1`

```
$ sed -i 's~REPLACE_WITH_HUB_NAMESPACE~cluster1~g' config/manager/manager.yaml
```

7. Replace the operator image and deploy the endpoint-metrics-operator:

```
$ make -f Makefile.prow deploy IMG=quay.io/<YOUR_USERNAME_IN_QUAY>/endpoint-metrics-operator:latest
```

8. Deploy the endpoint-metrics-operator CR:

```
$ kubectl -n open-cluster-management-addon-observability apply -f config/samples/observability.open-cluster-management.io_v1beta1_observabilityaddon.yaml
```

### Verify the Installation

After installed successfully, you will see the following pod are running:

```
# kubectl -n open-cluster-management-addon-observability get pod
NAME                                               READY   STATUS    RESTARTS   AGE
endpoint-observability-operator-7cf545f45c-cfjlk   1/1     Running   0          136m
metrics-collector-deployment-6dc9998cb-f2wd7       1/1     Running   0          136m
```

You should also see the CR created in the cluster:

```
# kubectl -n open-cluster-management-addon-observability get observabilityaddon
NAME                  AGE
observability-addon   137m
```

**Notice**: To deploy the `observabilityaddon` CR in local managed cluster just for dev/test purpose. In real topology, the `observabilityaddon` CR will be created in hub cluster, the endpoint-monitoring-operator should talk to api server of hub cluster to watch those CRs, and then perform changes on managed cluster. 

### View metrics in dashboard

Access Grafana console in hub cluster at https://{YOUR_DOMAIN}/grafana, view the metrics in the dashboard named "ACM:Managed Cluster Monitoring"

