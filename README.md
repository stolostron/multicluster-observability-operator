# multicluster-monitoring-operator
The guide is used for developer to build and install the multicluster-monitoring-operator. It can be running in [kind][install_kind] if you don't have a OCP environment.

### Prerequisites

- [git][git_tool]
- [go][go_tool] version v1.13.9+.
- [docker][docker_tool] version 19.03+.
- [kubectl][kubectl_tool] version v1.14+.
- Access to a Kubernetes v1.11.3+ cluster.

### Install the Operator SDK CLI

Follow the steps in the [installation guide][install_guide] to learn how to install the Operator SDK CLI tool. It requires version v0.15.1.
Or just use this command to download `operator-sdk` for Mac:
```
curl -L https://github.com/operator-framework/operator-sdk/releases/download/v0.15.1/operator-sdk-v0.15.1-x86_64-apple-darwin -o operator-sdk
```

### Build the Operator

- git clone this repository.
- `go mod vendor`
- `operator-sdk build <repo>/<component>:<tag>` for example: quay.io/multicluster-monitoring-operator:v0.1.0.
- Replace the image in `deploy/operator.yaml`.

### Deploy this Operator

1. Prepare the `StorageClass` and `PersistentVolume` to apply into the existing environment. For example:
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
    path: "/tmp/thanos/teamcitydata1"
```
2. Apply the manifests
```
kubectl apply -f deploy/crds/
kubectl apply -f deploy/req_crds/
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

[install_kind]: https://github.com/kubernetes-sigs/kind
[install_guide]: https://github.com/operator-framework/operator-sdk/blob/master/doc/user/install-operator-sdk.md
[git_tool]:https://git-scm.com/downloads
[go_tool]:https://golang.org/dl/
[docker_tool]:https://docs.docker.com/install/
[kubectl_tool]:https://kubernetes.io/docs/tasks/tools/install-kubectl/
