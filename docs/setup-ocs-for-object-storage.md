## Install the OpenShift Container Storage

Following this guide to install the OpenShift Container Storage to your cluster: https://access.redhat.com/documentation/en-us/red_hat_openshift_container_storage/4.4/html/deploying_openshift_container_storage/deploying-openshift-container-storage-on-openshift-container-platform_rhocs#installing-openshift-container-storage-operator-using-the-operator-hub_aws-vmware

## Accessing object storage configuration

Following this guide to access the relevant endpoint, access key, and secret access key: https://access.redhat.com/documentation/en-us/red_hat_openshift_container_storage/4.3/html/managing_openshift_container_storage/multicloud-object-gateway_rhocs#accessing-the-Multicloud-object-gateway-from-the-terminal_rhocs

- access key (`AWS_ACCESS_KEY_ID` value)

```
$ ACCESS_KEY=$(kubectl get secret noobaa-admin -n openshift-storage -o json | jq -r '.data.AWS_ACCESS_KEY_ID|@base64d')
$ echo $ACCESS_KEY
GZnv6sSHjHQMM3UrYqsn
```

- secret access key (`AWS_SECRET_ACCESS_KEY` value)

```
$ SECRET_KEY=$(kubectl get secret noobaa-admin -n openshift-storage -o json | jq -r '.data.AWS_SECRET_ACCESS_KEY|@base64d')
$ echo $SECRET_KEY
WZIfOGYLx1DvlyKC9BII99VnSzDDJwymMZR3vAtL
```

- endpoint

```
$ kubectl get noobaa -n openshift-storage -o yaml

...
      serviceS3:
        externalDNS:
        - https://s3-openshift-storage.apps.acm-hub.dev05.red-chesterfield.com
        - https://a916e5db6fa55485ba046a55c908147d-919567698.us-east-1.elb.amazonaws.com:443
...
```

Your object storage configuration should as following:

```
type: s3
config:
  bucket: first.bucket
  endpoint: a916e5db6fa55485ba046a55c908147d-919567698.us-east-1.elb.amazonaws.com
  insecure: true
  access_key: GZnv6sSHjHQMM3UrYqsn
  secret_key: WZIfOGYLx1DvlyKC9BII99VnSzDDJwymMZR3vAtL
```

Then you can be following these steps to deploy multicluster-observability-operator: https://github.com/open-cluster-management/multicluster-observability-operator#install-this-operator-on-rhacm
