## Install Ceph deployment

### Create the security context constraints

```
$ cd example/ceph/
$ oc apply -f scc.yaml --validate=false
```

### Deploy the rook operator

```
$ oc create -f operator.yaml
$ oc get pods -n rook-ceph-system
NAME                                  READY   STATUS    RESTARTS   AGE
rook-ceph-agent-4gxgw                 1/1     Running   0          18h
rook-ceph-agent-lhrv8                 1/1     Running   0          18h
rook-ceph-agent-mctzr                 1/1     Running   0          18h
rook-ceph-agent-qt8mb                 1/1     Running   0          18h
rook-ceph-agent-xt97w                 1/1     Running   0          18h
rook-ceph-agent-z59pv                 1/1     Running   0          18h
rook-ceph-operator-69c6dd8dd4-wvmnf   1/1     Running   0          18h
rook-discover-9pwgc                   1/1     Running   0          18h
rook-discover-bdffp                   1/1     Running   0          18h
rook-discover-cqx7h                   1/1     Running   0          18h
rook-discover-g4sl9                   1/1     Running   0          18h
rook-discover-k74tg                   1/1     Running   0          18h
rook-discover-wjfbz                   1/1     Running   0          18h
```

### Create rook cluster

```
$ oc create -f cluster.yaml
$ oc get pods -n rook-ceph
NAME                                          READY   STATUS      RESTARTS   AGE
rook-ceph-mgr-a-7d787567d-mlptf               1/1     Running     0          17h
rook-ceph-mon-a-745d4555bb-95rpp              1/1     Running     0          17h
rook-ceph-mon-b-6ff54dbb64-wr6hq              1/1     Running     0          17h
rook-ceph-mon-c-88b4f678-ws5l2                1/1     Running     0          17h
rook-ceph-osd-0-dfd69fcd-tlgzb                1/1     Running     0          17h
rook-ceph-osd-1-75569754d-5jwgx               1/1     Running     0          17h
rook-ceph-osd-2-7fbbfc698f-md7t4              1/1     Running     0          17h
rook-ceph-osd-3-856bdc77f8-958kx              1/1     Running     0          17h
rook-ceph-osd-4-7ff6c4c8c9-rqcfp              1/1     Running     0          17h
rook-ceph-osd-5-d7f4b95d4-qnklt               1/1     Running     0          17h
rook-ceph-osd-prepare-ip-10-0-138-240-kxlg7   0/2     Completed   0          17h
rook-ceph-osd-prepare-ip-10-0-141-127-h4jht   0/2     Completed   0          17h
rook-ceph-osd-prepare-ip-10-0-146-129-2ntjk   0/2     Completed   0          17h
rook-ceph-osd-prepare-ip-10-0-151-220-dtj78   0/2     Completed   0          17h
rook-ceph-osd-prepare-ip-10-0-161-103-b4xvr   0/2     Completed   0          17h
rook-ceph-osd-prepare-ip-10-0-170-222-rd4dv   0/2     Completed   0          17h
rook-ceph-rgw-object-5b586bd796-8hqzj         1/1     Running     0          17h
rook-ceph-tools                               1/1     Running     0          17h
```

### Verify your Ceph cluster

```
$ oc create -f toolbox.yaml
$ oc  -n rook-ceph exec -it rook-ceph-tools bash
[root@rook-ceph-tools /]# ceph -s
  cluster:
    id:     fd0a79e6-2332-42e9-a57b-f32153c7ffed
    health: HEALTH_OK

  services:
    mon: 3 daemons, quorum c,b,a
    mgr: a(active)
    osd: 6 osds: 6 up, 6 in
    rgw: 1 daemon active

  data:
    pools:   6 pools, 600 pgs
    objects: 339  objects, 223 MiB
    usage:   265 GiB used, 452 GiB / 717 GiB avail
    pgs:     600 active+clean
```

### Accessing S3 object storage

```
$ oc create -f object.yaml
$ oc create -f object-user.yaml
$ oc describe secret -n rook-ceph rook-ceph-object-user-object-object
Name:         rook-ceph-object-user-object-object
Namespace:    rook-ceph
Labels:       app=rook-ceph-rgw
              rook_cluster=rook-ceph
              rook_object_store=object
              user=object
Annotations:  <none>

Type:  kubernetes.io/rook

Data
====
AccessKey:  20 bytes
SecretKey:  40 bytes

```

- access key (`AWS_ACCESS_KEY_ID` value)

```
$ ACCESS_KEY=$(oc -n rook-ceph get secret rook-ceph-object-user-object-object -o yaml | grep AccessKey | awk '{print $2}' | base64 --decode)
$ echo $ACCESS_KEY
CDDQ0YU1C4A77A0GE54S
```

- secret access key (`AWS_SECRET_ACCESS_KEY` value)

```
$ SECRET_KEY=$(oc -n rook-ceph get secret rook-ceph-object-user-object-object -o yaml | grep SecretKey | awk '{print $2}' | base64 --decode)
$ echo $SECRET_KEY
awkEbItAs6OXsbOC6Qk7SX45h01GSw51z9SDasBI
```

- endpoint

```
$ oc -n rook-ceph expose svc/rook-ceph-rgw-object
$ oc -n rook-ceph get route | awk '{ print  $2 }'
HOST/PORT
rook-ceph-rgw-object-rook-ceph.apps.acm-hub.dev05.red-chesterfield.com
```

Your object storage configuration should as following:

```
type: s3
config:
  bucket: bucket_name # Need to create a bucket in advance
  endpoint: rook-ceph-rgw-object-rook-ceph.apps.acm-hub.dev05.red-chesterfield.com
  insecure: true
  access_key: CDDQ0YU1C4A77A0GE54S
  secret_key: awkEbItAs6OXsbOC6Qk7SX45h01GSw51z9SDasBI
```

Then you can be following these steps to deploy multicluster-observability-operator: https://github.com/open-cluster-management/multicluster-monitoring-operator#install-this-operator-on-rhacm