## rbac

- admin-rbac.yaml: assign `admin` to cluster manager
- user1-rbac.yaml: assign `user1` to `cluster1` manager
- user2-rbac.yaml: assign `user2` to `cluster2` manager

```
$ oc apply -f admin-rbac.yaml -f user1-rbac.yaml -f user2-rbac.yaml
$ oc apply -f cluster1.yaml -f cluster2.yaml
$ oc login -u admin -p admin
$ oc get managedclusters
NAME       AGE
cluster1   12m
cluster2   12m
$ oc login -u user1 -p user1
$ oc get managedclusters cluster1
NAME       AGE
cluster1   16m
$ oc login -u user2 -p user2
$ oc get managedclusters cluster2
NAME       AGE
cluster2   16m
```