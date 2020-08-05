# Object Storage

The multicluster-observability-operator supports S3-compatible object storage. Current object storage configurations:

```yaml
apiVersion: observability.open-cluster-management.io/v1alpha1
kind: MultiClusterObservability
metadata:
  name: observability
spec:
  availabilityConfig: Basic
  imagePullPolicy: Always
  imagePullSecret: multiclusterhub-operator-pull-secret
  retentionResolution1h: 30d
  retentionResolution5m: 14d
  retentionResolutionRaw: 5d
  storageClass: gp2
  storageSize: 50Gi
```

- type: type of object storage backend server, currently only `minio` and `s3` are supported
- bucket: object storage bucket name
- endpoint: object storage server endpoint
- insecure: configure object storage server use HTTP or HTTPs
- access_key: object storage server access key
- secret_key: object storage server secret key
- storage:  minio local PVC storage size, just for minio only, ignore it if type is s3


By default, you do not need to configure `objectStorageConfig` field. The multicluster-observability-operator will automatically install [Minio](https://min.io/) as backend object storage server, you can also configure an S3 bucket as an object store.

## Minio (default)

When you install [multicluster-observability-operator CR](/deploy/crds/observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml), it will use the following YAML content to configure Minio as default object storage server.

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

If you want to configure S3 as object store server, you can add the following YAML content to [multicluster-observability-operator CR](/deploy/crds/observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml):

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
