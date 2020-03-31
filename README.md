# multicluster-monitoring-operator
Operator for monitoring

### How to build the Operator

1. Install `go` version 1.13
2. Download operator-sdk
`curl -L https://github.com/operator-framework/operator-sdk/releases/download/v0.15.1/operator-sdk-v0.15.1-x86_64-apple-darwin -o operator-sdk`
3. git clone this repo
4. `operator-sdk build <repo>/<component>:<tag>` for example: clyang/multicluster-monitoring-operator:v0.1.0. I will request to create a quay.io repo
5. replace the image in `deploy/operator.yaml`

### How to use this Operator

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
oc apply -f deploy/crds/monitoring.open-cluster-management.io_multiclustermonitorings_crd.yaml
oc apply -f deploy/req_crds/
oc apply -f deploy/
oc apply -f deploy/crds/monitoring.open-cluster-management.io_v1_multiclustermonitoring_cr.yaml

```
