## Configuring Observability for Finer-Grained Access Control 

This feature provides the ability to control access to metrics collected from managed clusters at a namespace level granularity. The existing mechanism allows access control per managed cluster, this level of granularity is not sufficient when managed clusters are of large size and are shared by multiple teams or applications in the organization. In such cases, each application team's access should be restricted to just their application's metrics  and should not be able access metrics from other applications that share the same cluster.  Namespace level granularty enables limiting access to specific namespaces on the clusters thereby enabling application team's metrics access to be restricted to only the namespaces where their application is provisioned.

Metrics access control is configured on the hub-cluster for hub users. Every managed-cluster is represented by a  `ManagedCluster` custom resource object on the hub-cluster, RBAC is specified through rules set up on these managedcluster resources and action verbs that indicate the namespaces allowed. For e.g. the verb

1. "metrics/test" specifies access to metrics collected from namespace "test" 
2. "metrics/*" specifies access to to metrics from all namespaces on the managedcluster, this is the only regex supported.

The existing mechanism of managed-cluster level access control, where access to all metrics from a  managed cluster is configured through binding of 'admin' role  on the managedcluster's project on the hub-cluster, is also still supported for backward compatibility. However, when both cluster level and namespace level RBAC is configured for a given user then namespace level RBAC takes precedence, i.e if user has both  'admin' role access to managed-cluster's project and "metrics/test" access to the same managed-cluster resource then the user can only access metrics from the 'test' namespace on that managed-cluster.

MCO images built through the source in this repo, specifically the rbac_query_proxy image, will allow the user to preview this more granular access control feature.

## Cluster roles and bindings for granular metrics access

The following describes the definitions of role and role binding objects for metrics access in more detail.

Consider an example scenario where an application App-Red is provisioned to

* Clusters devcluster1, devcluster2  
* Namespaces AppRedNs1 and AppRedNs2 on both the above clusters

An App-Red-Admins user group is created for the  users who are  allowed to administer this application.

### User group for App-Red Admins

```yaml
---
kind: Group
apiVersion: user.openshift.io/v1
metadata:
 name: app-red-admins
users:
 - reduser1
 - reduser2
---
```

RBAC configuration to give these admins of application App-Red access to view metrics that belong their application  i.e metrics from the namespaces (AppRedNs1, AppRedNs2) on clusters (devcluster1, devcluster2) is as simple as below


### ClusterRole with permissions to access metrics from application App-Red

```yaml
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
 name: cluster-red-metrics
rules:
 - apiGroups:
     - "cluster.open-cluster-management.io"
   resources:
     - managedclusters
   resourceNames:
     - devcluster1
     - devcluster2
   verbs:
     - metrics/AppRedNs1
     - metrics/AppRedNs2
---
```

### ClusterRoleBinding for assigning App-Red metrics access to App-Red admins

```yaml
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
 name: app-red-metric-viewers
subjects:
 - kind: Group
   apiGroup: rbac.authorization.k8s.io
   name: app-red-admins
roleRef:
 apiGroup: rbac.authorization.k8s.io
 kind: ClusterRole
 name: app-red-metrics
---
```
