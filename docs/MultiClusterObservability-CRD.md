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
   <td>EnableDownsampling
   </td>
   <td>bool
   </td>
   <td>Enable or disable the downsample.
<p>
The default value is <strong>true</strong>.
<p>
Note: Disabling downsampling is not recommended as querying long time ranges without non-downsampled data is not efficient and useful.
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>ImagePullSecret
   </td>
   <td>string
   </td>
   <td>Pull secret of the MultiCluster Observability images
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>NodeSelector
   </td>
   <td>map[string]string
   </td>
   <td>Spec of NodeSelector
   </td>
   <td>N
   </td>
  </tr>  
  <tr>
   <td>ObservabilityAddonSpec
   </td>
   <td>ObservabilityAddOnSpec
   </td>
   <td>The observabilityAddonSpec defines the global settings for all managed clusters which have observability add-on enabled.
   </td>
   <td>Y
   </td>
  </tr>
  <tr>
   </td>
   <td>RetentionConfig
   </td>
   <td>RetentionConfig
   </td>
   <td>Specifies the data retention configurations to be used by Observability
   </td>
   <td>Y
   </td>
  </tr>
  <tr>
   </td>
   <td>StorageConfig
   </td>
   <td>StorageConfig
   </td>
   <td>Specifies the storage configuration to be used by Observability
   </td>
   <td>Y
   </td>
  </tr>  
  <tr>
   <td>Tolerations
   </td>
   <td>[]corev1.Toleration
   </td>
   <td>Tolerations causes all components to tolerate any taints
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
   <td>BlockDuration
   </td>
   <td>string
   </td>
   <td>configure --tsdb.block-duration in rule (Block duration for TSDB block)
<p>
Default is 2h
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>CleanupInterval
   </td>
   <td>string
   </td>
   <td>Configure --compact.cleanup-interval in compact. How often we should clean up partially uploaded blocks and blocks with deletion mark in the background when --wait has been enabled. Setting it to "0s" disables it
<p>
Default is 5m
   </td>
   <td>N
   </td>
  </tr>
    <tr>
   <td>DeleteDelay
   </td>
   <td>string
   </td>
   <td>configure --delete-delay in compact Time before a block marked for deletion is deleted from bucket.
<p>
Default is 48h
   </td>
   <td>N
   </td>
  </tr>
    <tr>
   <td>RetentionInLocal
   </td>
   <td>string
   </td>
   <td>How long to retain raw samples in a local disk. It applies to rule/receive: --tsdb.retention in receive --tsdb.retention in rule.
<p>
Default is 4d
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>RetentionResolutionRaw
   </td>
   <td>string
   </td>
   <td>How long to retain raw samples in a bucket.
<p>
Default is 5d
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>RetentionResolution5m
   </td>
   <td>string
   </td>
   <td>How long to retain samples of resolution 1 (5 minutes) in a bucket.
<p>
 Default is 14d
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>RetentionResolution1h
   </td>
   <td>string
   </td>
   <td>How long to retain samples of resolution 2 (1 hour) in a bucket.
<p>
Default is 30d.
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
   <td>AlertmanagerStorageSize
   </td>
   <td>String
   </td>
   <td>The amount of storage applied to alertmanager stateful sets.
<p>
The default is 1Gi
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>CompactStorageSize
   </td>
   <td>String
   </td>
   <td>The amount of storage applied to thanos compact stateful sets.
<p>
The default is 100Gi
   </td>
   <td>N
   </td>
  </tr>  <tr>
   <td>MetricObjectStorage
   </td>
   <td>PreConfiguredStorage
   </td>
   <td>Reference to Preconfigured Storage to be used by Observability.
   </td>
   <td>Y
   </td>
  </tr>
  <tr>
   <td>ReceiveStorageSize
   </td>
   <td>String
   </td>
   <td>The amount of storage applied to thanos receive stateful sets.
<p>
The default is 100Gi
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>RuleStorageSize
   </td>
   <td>String
   </td>
   <td>The amount of storage applied to thanos rule stateful sets.
<p>
The default is 1Gi
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>StorageClass
   </td>
   <td>String
   </td>
   <td>Specify the storageClass Stateful Sets.  This storage class will also be used for Object Storage if MetricObjectStorage was configured for the system to create the storage.
<p>
The default  is gp2.
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>StoreStorageSize
   </td>
   <td>String
   </td>
   <td>The amount of storage applied to thanos store stateful sets.
<p>
The default is 10Gi
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
   <td>Key
   </td>
   <td>string
   </td>
   <td>The key of the secret to select from. Must be a valid secret key. Refer to <a href="https://thanos.io/storage.md/#configuration">https://thanos.io/storage.md/#configuration</a> for a valid content of key.
   </td>
   <td>y
   </td>
  </tr>
  <tr>
   <td>Name
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
   <td>EnableMetrics
   </td>
   <td>bool
   </td>
   <td>Push metrics or not
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>Interval
   </td>
   <td>int32
   </td>
   <td>Interval for the metrics collector push metrics to hub server
<p>
The default is 1m
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
