// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package v1beta2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MultiClusterObservabilitySpec defines the desired state of MultiClusterObservability
type MultiClusterObservabilitySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Enable or disable the downsample.
	// The default value is true.
	// This is not recommended as querying long time ranges
	// without non-downsampled data is not efficient and useful.
	// +kubebuilder:default:=true
	EnableDownsampling bool `json:"enableDownsampling,omitempty"`
	// Pull policy of the MultiClusterObservability images
	// +optional
	// +kubebuilder:default:=Always
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// Pull secret of the MultiClusterObservability images
	// +optional
	// +kubebuilder:default:=multiclusterhub-operator-pull-secret
	ImagePullSecret string `json:"imagePullSecret,omitempty"`
	// Spec of NodeSelector
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations causes all components to tolerate any taints.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// The spec of the data retention configurations
	// +required
	RetentionConfig *RetentionConfig `json:"retentionConfig,omitempty"`
	// Specifies the storage to be used by Observability
	// +required
	StorageConfig *StorageConfigObject `json:"storageConfigObject,omitempty"`
	// The ObservabilityAddonSpec defines the global settings for all managed
	// clusters which have observability add-on enabled.
	// +optional
	ObservabilityAddonSpec *ObservabilityAddonSpec `json:"observabilityAddonSpec,omitempty"`
}

// ObservabilityAddonSpec is the spec of observability addon
type ObservabilityAddonSpec struct {
	// EnableMetrics indicates the observability addon push metrics to hub server.
	// +optional
	// +kubebuilder:default:=true
	EnableMetrics bool `json:"enableMetrics,omitempty"`
	// Interval for the observability addon push metrics to hub server.
	// +optional
	// +kubebuilder:default:=30
	// +kubebuilder:validation:Minimum=15
	// +kubebuilder:validation:Maximum=3600
	Interval int32 `json:"interval,omitempty"`
}

// RetentionConfig is the spec of retention configurations.
type RetentionConfig struct {
	// How long to retain raw samples in a bucket.
	// It applies to --retention.resolution-raw in compact.
	// +optional
	// +kubebuilder:default:="5d"
	RetentionResolutionRaw string `json:"retentionResolutionRaw,omitempty"`
	// How long to retain samples of resolution 1 (5 minutes) in bucket.
	// +optional
	// +kubebuilder:default:="14d"
	RetentionResolution5m string `json:"retentionResolution5m,omitempty"`
	// How long to retain samples of resolution 2 (1 hour) in bucket.
	// +optional
	// +kubebuilder:default:="30d"
	RetentionResolution1h string `json:"retentionResolution1h,omitempty"`
	// How long to retain raw samples in a local disk. It applies to rule/receive:
	// --tsdb.retention in receive
	// --tsdb.retention in rule
	// +optional
	// +kubebuilder:default:="4d"
	RetentionInLocal string `json:"retentionInLocal,omitempty"`
	// Configure --compact.cleanup-interval in compact.
	// How often we should clean up partially uploaded blocks and
	// blocks with deletion mark in the background when --wait has been enabled.
	// Setting it to "0s" disables it
	// +optional
	// +kubebuilder:default:="5m"
	CleanupInterval string `json:"cleanupInterval,omitempty"`
	// configure --delete-delay in compact
	// Time before a block marked for deletion is deleted from bucket.
	// +optional
	// +kubebuilder:default:="48h"
	DeleteDelay string `json:"deleteDelay,omitempty"`
	// configure --tsdb.block-duration in rule (Block duration for TSDB block)
	// +optional
	// +kubebuilder:default:="2h"
	BlockDuration string `json:"blockDuration,omitempty"`
}

// StorageConfigObject is the spec of object storage.
type StorageConfigObject struct {
	// Object store config secret for metrics
	// +required
	MetricObjectStorage *PreConfiguredStorage `json:"metricObjectStorage,omitempty"`
	// Specify the storageClass Stateful Sets. This storage class will also
	// be used for Object Storage if MetricObjectStorage was configured for
	// the system to create the storage.
	// +optional
	// +kubebuilder:default:=gp2
	StorageClass string `json:"storageClass,omitempty"`
	// The amount of storage applied to alertmanager stateful sets,
	// +optional
	// +kubebuilder:default:="1Gi"
	AlertmanagerStorageSize string `json:"alertmanagerStorageSize,omitempty"`
	// The amount of storage applied to thanos rule stateful sets,
	// +optional
	// +kubebuilder:default:="1Gi"
	RuleStorageSize string `json:"ruleStorageSize,omitempty"`
	// The amount of storage applied to thanos compact stateful sets,
	// +optional
	// +kubebuilder:default:="100Gi"
	CompactStorageSize string `json:"compactStorageSize,omitempty"`
	// The amount of storage applied to thanos receive stateful sets,
	// +optional
	// +kubebuilder:default:="100Gi"
	ReceiveStorageSize string `json:"receiveStorageSize,omitempty"`
	// The amount of storage applied to thanos store stateful sets,
	// +optional
	// +kubebuilder:default:="10Gi"
	StoreStorageSize string `json:"storeStorageSize,omitempty"`
}

type PreConfiguredStorage struct {
	// The key of the secret to select from. Must be a valid secret key.
	// Refer to https://thanos.io/storage.md/#configuration for a valid content of key.
	// +required
	Key string `json:"key,omitempty"`
	// Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// +required
	Name string `json:"name,omitempty"`
}

// MultiClusterObservabilityStatus defines the observed state of MultiClusterObservability
type MultiClusterObservabilityStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Represents the status of each deployment
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

// Condition is from metav1.Condition.
// Cannot use it directly because the upgrade issue.
// Have to mark LastTransitionTime and Status as optional.
type Condition struct {
	// type of condition in CamelCase or in foo.example.com/CamelCase.
	// ---
	// Many .condition.type values are consistent across resources like Available, but because arbitrary conditions can be
	// useful (see .node.status.conditions), the ability to deconflict is important.
	// The regex it matches is (dns1123SubdomainFmt/)?(qualifiedNameFmt)
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
	// +kubebuilder:validation:MaxLength=316
	Type string `json:"type" protobuf:"bytes,1,opt,name=type"`
	// status of the condition, one of True, False, Unknown.
	// +optional
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=True;False;Unknown
	Status metav1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status"`
	// observedGeneration represents the .metadata.generation that the condition was set based upon.
	// For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9,
	// the condition is out of date
	// with respect to the current state of the instance.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,3,opt,name=observedGeneration"`
	// lastTransitionTime is the last time the condition transitioned from one status to another.
	// This should be when the underlying condition changed.  If that is not known, then using the time when the API
	// field changed is acceptable.
	// +optional
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=date-time
	LastTransitionTime metav1.Time `json:"lastTransitionTime" protobuf:"bytes,4,opt,name=lastTransitionTime"`
	// reason contains a programmatic identifier indicating the reason for the condition's last transition.
	// Producers of specific condition types may define expected values and meanings for this field,
	// and whether the values are considered a guaranteed API.
	// The value should be a CamelCase string.
	// This field may not be empty.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=1024
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$`
	Reason string `json:"reason" protobuf:"bytes,5,opt,name=reason"`
	// message is a human readable message indicating details about the transition.
	// This may be an empty string.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=32768
	Message string `json:"message" protobuf:"bytes,6,opt,name=message"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// MultiClusterObservability defines the configuration for the Observability installation on
// Hub and Managed Clusters all through this one custom resource.
// +kubebuilder:pruning:PreserveUnknownFields
// +kubebuilder:resource:path=multiclusterobservabilities,scope=Cluster,shortName=mco
type MultiClusterObservability struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MultiClusterObservabilitySpec   `json:"spec,omitempty"`
	Status MultiClusterObservabilityStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// MultiClusterObservabilityList contains a list of MultiClusterObservability
type MultiClusterObservabilityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MultiClusterObservability `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MultiClusterObservability{}, &MultiClusterObservabilityList{})
}
