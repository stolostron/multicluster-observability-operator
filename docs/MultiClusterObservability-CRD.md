# MultiClusterObservability CRD

## Description

MultiClusterObservability API is the interface to manage the MultiClusterObservability Operator which deploys and manages the Observability components on the RHACM Hub Cluster. MultiClusterObservability is a cluster scoped CRD. The short name is MCO.

## API Version

observability.open-cluster-management.io/v1beta2


## Specification


<table>
  <tr>
   <td><strong>Property</strong>
   </td>
   <td><strong>Type</strong>
   </td>
   <td><strong>Description</strong>
   </td>
   <td><strong>Req’d</strong>
   </td>
  </tr>
  <tr>
   <td>enableDownsampling
   </td>
   <td>bool
   </td>
   <td>Enable or disable the downsampling.
<p>
The default is <strong>true</strong>.
<p>
Note: Disabling downsampling is not recommended as querying long time ranges without non-downsampled data is not efficient and useful.
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>imagePullPolicy
   </td>
   <td>corev1.PullPolicy
   </td>
   <td>Pull policy of the MultiClusterObservability images. The default is <strong>Always<strong>.
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>imagePullSecret
   </td>
   <td>string
   </td>
   <td>Pull secret of the MultiCluster Observability images. The default is <strong>multiclusterhub-operator-pull-secret</strong>
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>nodeSelector
   </td>
   <td>map[string]string
   </td>
   <td>Spec of NodeSelector
   </td>
   <td>N
   </td>
  </tr>  
  <tr>
   <td>observabilityAddonSpec
   </td>
   <td>ObservabilityAddOnSpec
   </td>
   <td>The observabilityAddonSpec defines the global settings for all managed clusters which have observability add-on installed.
   </td>
   <td>Y
   </td>
  </tr>
  <tr>
   </td>
   <td>storageConfig
   </td>
   <td>StorageConfig
   </td>
   <td>Specifies the storage configuration to be used by Observability
   </td>
   <td>Y
   </td>
  </tr>  
  <tr>
   <td>tolerations
   </td>
   <td>[]corev1.Toleration
   </td>
   <td>Tolerations causes all components to tolerate any taints
   </td>
   <td>N
   </td>
  </tr> 
  <tr>
   </td>
   <td>advanced
   </td>
   <td>AdvancedConfig
   </td>
   <td>Advanced configurations for observability 
   </td>
   <td>N
   </td>
  </tr>
</table>

### StorageConfig

<table>
  <tr>
   <td><strong>Property</strong>
   </td>
   <td><strong>Type</strong>
   </td>
   <td><strong>Description</strong>
   </td>
   <td><strong>Req’d</strong>
   </td>
  </tr>
  <tr>
   <td>alertmanagerStorageSize
   </td>
   <td>String
   </td>
   <td>The amount of storage applied to alertmanager stateful sets.
<p>
The default is <strong>1Gi</strong>
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>compactStorageSize
   </td>
   <td>String
   </td>
   <td>The amount of storage applied to thanos compact stateful sets.
<p>
The default is <strong>100Gi</strong>
   </td>
   <td>N
   </td>
  </tr>  <tr>
   <td>metricObjectStorage
   </td>
   <td>PreConfiguredStorage
   </td>
   <td>Reference to Preconfigured Storage to be used by Observability.
   </td>
   <td>Y
   </td>
  </tr>
  <tr>
   <td>receiveStorageSize
   </td>
   <td>String
   </td>
   <td>The amount of storage applied to thanos receive stateful sets.
<p>
The default is <strong>100Gi</strong>
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>ruleStorageSize
   </td>
   <td>String
   </td>
   <td>The amount of storage applied to thanos rule stateful sets.
<p>
The default is <strong>1Gi</strong>
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>storageClass
   </td>
   <td>String
   </td>
   <td>Specify the storageClass Stateful Sets.  This storage class will also be used for Object Storage if MetricObjectStorage was configured for the system to create the storage.
<p>
The default is <strong>gp2</strong>.
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>storeStorageSize
   </td>
   <td>String
   </td>
   <td>The amount of storage applied to thanos store stateful sets.
<p>
The default is <strong>10Gi</strong>
   </td>
   <td>N
   </td>
  </tr>
</table>

### PreConfiguredStorage

<table>
  <tr>
   <td><strong>Property</strong>
   </td>
   <td><strong>Type</strong>
   </td>
   <td><strong>Description</strong>
   </td>
   <td><strong>Req’d</strong>
   </td>
  </tr>
  <tr>
   <td>key
   </td>
   <td>string
   </td>
   <td>The key of the secret to select from. Must be a valid secret key. Refer to <a href="https://thanos.io/storage.md/#configuration">https://thanos.io/storage.md/#configuration</a> for a valid content of key.
   </td>
   <td>y
   </td>
  </tr>
  <tr>
   <td>name
   </td>
   <td>string
   </td>
   <td>Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
   </td>
   <td>y
   </td>
  </tr>
</table>

### ObservabilityAddonSpec

<table>
  <tr>
   <td><strong>Property</strong>
   </td>
   <td><strong>Type</strong>
   </td>
   <td><strong>Description</strong>
   </td>
   <td><strong>Req’d</strong>
   </td>
  </tr>
  <tr>
   <td>enableMetrics
   </td>
   <td>bool
   </td>
   <td>Push metrics or not
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>interval
   </td>
   <td>int32
   </td>
   <td>Interval for the metrics collector push metrics to hub server.
<p>
The default is <strong>1m</strong>
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>resources
   </td>
   <td>corev1.ResourceRequirements
   </td>
   <td>Resource for the metrics collector resource requirement.
<p>
The default CPU request is 100m, memory request is 100Mi. The default CPU limit is 100m, memory limit is 600Mi.
   </td>
   <td>N
   </td>
  </tr>
</table>

### AdvancedConfig

<table>
  <tr>
   <td><strong>Property</strong>
   </td>
   <td><strong>Type</strong>
   </td>
   <td><strong>Description</strong>
   </td>
   <td><strong>Req’d</strong>
   </td>
  </tr>
  <tr>
   </td>
   <td>retentionConfig
   </td>
   <td>RetentionConfig
   </td>
   <td>Specifies the data retention configurations to be used by Observability
   </td>
   <td>Y
   </td>
  </tr>
  <tr>
   <td>rbacQueryProxy
   </td>
   <td>CommonSpec
   </td>
   <td>Specifies the replicas, resources for rbac-query-proxy deployment.
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>grafana
   </td>
   <td>CommonSpec
   </td>
   <td>Specifies the replicas, resources for grafana deployment.
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>alertmanager
   </td>
   <td>CommonSpec
   </td>
   <td>Specifies the replicas, resources for alertmanager statefulset.
   </td>
   <td>N
   </td>
  </tr> 
  <tr>
   <td>observatoriumAPI
   </td>
   <td>CommonSpec
   </td>
   <td>Specifies the replicas, resources for observatorium-api deployment.
   </td>
   <td>N
   </td>
  </tr> 
  <tr>
   <td>queryFrontend
   </td>
   <td>CommonSpec
   </td>
   <td>Specifies the replicas, resources for query-frontend deployment.
   </td>
   <td>N
   </td>
  </tr> 
  <tr>
   <td>query
   </td>
   <td>CommonSpec
   </td>
   <td>Specifies the replicas, resources for query deployment.
   </td>
   <td>N
   </td>
  </tr> 
  <tr>
   <td>receive
   </td>
   <td>CommonSpec
   </td>
   <td>Specifies the replicas, resources for receive statefulset.
   </td>
   <td>N
   </td>
  </tr> 
  <tr>
   <td>rule
   </td>
   <td>CommonSpec
   </td>
   <td>Specifies the replicas, resources for rule statefulset.
   </td>
   <td>N
   </td>
  </tr> 
  <tr>
   <td>store
   </td>
   <td>CommonSpec
   </td>
   <td>Specifies the replicas, resources for store statefulset.
   </td>
   <td>N
   </td>
  </tr> 
  <tr>
   <td>compact
   </td>
   <td>CompactSpec
   </td>
   <td>Specifies the resources for compact statefulset.
   </td>
   <td>N
   </td>
  </tr> 
  <tr>
   <td>storeMemcached
   </td>
   <td>CacheConfig
   </td>
   <td>Specifies the replicas, resources, etc for store-memcached.
   </td>
   <td>N
   </td>
  </tr> 
  <tr>
   <td>queryFrontendMemcached
   </td>
   <td>CacheConfig
   </td>
   <td>Specifies the replicas, resources, etc for query-frontend-memcached.
   </td>
   <td>N
   </td>
  </tr> 
</table>

### RetentionConfig

<table>
  <tr>
   <td><strong>Property</strong>
   </td>
   <td><strong>Type</strong>
   </td>
   <td><strong>Description</strong>
   </td>
   <td><strong>Req’d</strong>
   </td>
  </tr>
  <tr>
   <td>blockDuration
   </td>
   <td>string
   </td>
   <td>configure --tsdb.block-duration in rule (Block duration for TSDB block)
<p>
Default is <strong>2h</strong>
   </td>
   <td>N
   </td>
    <tr>
   <td>deleteDelay
   </td>
   <td>string
   </td>
   <td>configure --delete-delay in compact Time before a block marked for deletion is deleted from bucket.
<p>
Default is <strong>48h</strong>
   </td>
   <td>N
   </td>
  </tr>
    <tr>
   <td>retentionInLocal
   </td>
   <td>string
   </td>
   <td>How long to retain raw samples in a local disk. It applies to rule/receive: --tsdb.retention in receive --tsdb.retention in rule.
<p>
Default is <strong>24h</strong>.
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>retentionResolutionRaw
   </td>
   <td>string
   </td>
   <td>How long to retain raw samples in a bucket.
<p>
Default is <strong>30d</strong>.
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>retentionResolution5m
   </td>
   <td>string
   </td>
   <td>How long to retain samples of resolution 1 (5 minutes) in a bucket.
<p>
 Default is <strong>180d</strong>
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>retentionResolution1h
   </td>
   <td>string
   </td>
   <td>How long to retain samples of resolution 2 (1 hour) in a bucket.
<p>
Default is <strong>0d<strong>.
   </td>
   <td>N
   </td>
  </tr>
</table>

### CommonSpec

<table>
  <tr>
   <td><strong>Property</strong>
   </td>
   <td><strong>Type</strong>
   </td>
   <td><strong>Description</strong>
   </td>
   <td><strong>Req’d</strong>
   </td>
  </tr>
  <tr>
   <td>resources
   </td>
   <td>corev1.ResourceRequirements
   </td>
   <td>Compute Resources required by this component.
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>replicas
   </td>
   <td>int32
   </td>
   <td>Replicas for this component.
   </td>
   <td>N
   </td>
  </tr>
  </table>

### CompactSpec

<table>
  <tr>
   <td><strong>Property</strong>
   </td>
   <td><strong>Type</strong>
   </td>
   <td><strong>Description</strong>
   </td>
   <td><strong>Req’d</strong>
   </td>
  </tr>
  <tr>
   <td>resources
   </td>
   <td>corev1.ResourceRequirements
   </td>
   <td>Compute Resources required by this component.
   </td>
   <td>N
   </td>
  </tr>
  </table>

### CacheConfig

<table>
  <tr>
   <td><strong>Property</strong>
   </td>
   <td><strong>Type</strong>
   </td>
   <td><strong>Description</strong>
   </td>
   <td><strong>Req’d</strong>
   </td>
  </tr>
  <tr>
   <td>resources
   </td>
   <td>corev1.ResourceRequirements
   </td>
   <td>Compute Resources required by this component.
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>replicas
   </td>
   <td>int32
   </td>
   <td>Replicas for this component.
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>memoryLimitMb
   </td>
   <td>int32
   </td>
   <td>Memory limit of Memcached in megabytes.
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>maxItemSize
   </td>
   <td>string
   </td>
   <td>Max item size of Memcached (default: 1m, min: 1k, max: 1024m).
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>connectionLimit
   </td>
   <td>int32
   </td>
   <td>Max simultaneous connections of Memcached.
   </td>
   <td>N
   </td>
  </tr>

  </table>


### MultiClusterObservability Status

<table>
  <tr>
   <td><strong>Name</strong>
   </td>
   <td><strong>Description</strong>
   </td>
   <td><strong>Required</strong>
   </td>
   <td><strong>Default</strong>
   </td>
   <td><strong>Schema</strong>
   </td>
  </tr>
  <tr>
   <td>Status
   </td>
   <td>Status contains the different condition statuses for this deployment
   </td>
   <td>n/a
   </td>
   <td>[]
   </td>
   <td>metav1.Condition
   </td>
  </tr>
</table>
