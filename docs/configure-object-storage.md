# Object Storage

The multicluster-monitoring-operator supports S3-compatible object storage. Current object storage configurations:

```yaml
apiVersion: monitoring.open-cluster-management.io/v1alpha1
kind: MultiClusterMonitoring
metadata:
  name: monitoring
spec:
  version: latest
  imageRepository: "quay.io/open-cluster-management"
  imageTagSuffix: ""
  imagePullPolicy: Always
  imagePullSecret: quay-secret
  storageClass: gp2
  objectStorageConfig:
    type: minio
    config:
      bucket: thanos
      endpoint: minio:9000
      insecure: true
      access_key: minio
      secret_key: minio123
      storage: 1Gi
```

- type: type of object storage backend server, currently only `minio` and `s3` are supported
- bucket: object storage bucket name
- endpoint: object storage server endpoint
- insecure: configure object storage server use HTTP or HTTPs
- access_key: object storage server access key
- secret_key: object storage server secret key
- storage:  minio local PVC storage size, just for minio only, ignore it if type is s3


By default, you do not need to configure `objectStorageConfig` field. The multicluster-monitoring-operator will automatically install [Minio](https://min.io/) as backend object storage server, you can also configure an S3 bucket as an object store.

## Minio (default)

When you install [multicluster-monitoring-operator CR](/deploy/crds/monitoring.open-cluster-management.io_v1alpha1_multiclustermonitoring_cr.yaml), it will use the following YAML content to configure Minio as default object storage server.

```yaml
  objectStorageConfig:
    type: minio
    config:
      bucket: thanos
      endpoint: minio:9000
      insecure: true
      access_key: minio
      secret_key: minio123
      storage: 1Gi
```

## S3

If you want to configure S3 as object store server, you can add the following YAML content to [multicluster-monitoring-operator CR](/deploy/crds/monitoring.open-cluster-management.io_v1alpha1_multiclustermonitoring_cr.yaml):

```yaml
  objectStorageConfig:
    type: s3
    config:
      bucket: ""
      endpoint: ""
      insecure: false
      access_key: ""
      secret_key: ""
```
