# MultiClusterObservability CRD

## Description

MultiClusterObservability API is the interface to manage the MultiClusterObservability Operator which deploys and manages the Observability components on the RHACM Hub Cluster. MultiClusterObservability is a cluster scoped CRD. The short name is MCO.

## API Version

observability.open-cluster-management.io/v1beta1


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
   <td>ImagePullSecret
   </td>
   <td>string
   </td>
   <td>Pull secret of the MultiCluster Observability images
   </td>
   <td>n
   </td>
  </tr>
  <tr>
   <td>StorageConfig
   </td>
   <td>StorageConfigObject
   </td>
   <td>Specifies the storage to be used by Observability
   </td>
   <td>
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
  <tr>
   <td>AvailabilityConfig
   </td>
   <td>AvailabilityType
   </td>
   <td>ReplicaCount for HA support. Does not affect data stores. High will enable HA support. This will provide better support in cases of failover but consumes more resources.
<p>
Options are: Basic and High (default).
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
   <td>n
   </td>
  </tr>
</table>


### StorageConfigObject


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
   <td>MetricObjectStorage
   </td>
   <td>PreConfiguredStorage
   </td>
   <td>Reference to Preconfigured Storage to be used by Observability.
   </td>
   <td>N
   </td>
  </tr>
  <tr>
   <td>StatefulSetSize
   </td>
   <td>String
   </td>
   <td>The amount of storage applied to the Observability stateful sets, i.e. Thanos store, Rule, compact and receiver.
<p>
The default is 10Gi
   </td>
   <td>
   </td>
  </tr>
  <tr>
   <td>StatefulSetStorageClass
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
   <td><strong>name</strong>
   </td>
   <td><strong>description</strong>
   </td>
   <td><strong>required</strong>
   </td>
   <td><strong>default</strong>
   </td>
   <td><strong>schema</strong>
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
   <td>[]Conditions
   </td>
  </tr>
</table>


### Conditions

<table>
  <tr>
   <td><strong>type</strong>
   </td>
   <td><strong>reason</strong>
   </td>
   <td><strong>message</strong>
   </td>
  </tr>
  <tr>
   <td>Ready
   </td>
   <td>Ready
   </td>
   <td>Observability components deployed and running.
   </td>
  </tr>
  <tr>
   <td>Failed
   </td>
   <td>Failed
   </td>
   <td>Deployment failed for one or more components.
   </td>
  </tr>
</table>

