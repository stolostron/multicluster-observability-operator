// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AvailabilityType ...
type AvailabilityType string

const (
	// HABasic stands up most app subscriptions with a replicaCount of 1
	HABasic AvailabilityType = "Basic"
	// HAHigh stands up most app subscriptions with a replicaCount of 2
	HAHigh AvailabilityType = "High"
)

// MultiClusterObservabilitySpec defines the desired state of MultiClusterObservability
type MultiClusterObservabilitySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ReplicaCount for HA support. Does not affect data stores.
	// Enabled will toggle HA support. This will provide better support in cases of failover
	// but consumes more resources. Options are: Basic and High (default).
	// +optional
	// +kubebuilder:default:=High
	AvailabilityConfig AvailabilityType `json:"availabilityConfig,omitempty"`

	// Enable or disable the downsample.
	// The default value is false.
	// This is not recommended as querying long time ranges
	// without non-downsampled data is not efficient and useful.
	// +kubebuilder:default:=false
	EnableDownSampling bool `json:"enableDownSampling,omitempty"`

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

	// How long to retain raw samples in a bucket.
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

	// Specifies the storage to be used by Observability
	// +required
	StorageConfig *StorageConfigObject `json:"storageConfigObject,omitempty"`

	// The ObservabilityAddonSpec defines the global settings for all managed
	// clusters which have observability add-on enabled.
	// +optional
	ObservabilityAddonSpec *ObservabilityAddonSpec `json:"observabilityAddonSpec,omitempty"`
}

// StorageConfigObject is the spec of object storage.
type StorageConfigObject struct {
	// Object store config secret for metrics
	// +required
	MetricObjectStorage *PreConfiguredStorage `json:"metricObjectStorage,omitempty"`
	// The amount of storage applied to the Observability stateful sets, i.e.
	// Thanos store, Rule, compact and receiver.
	// +optional
	// +kubebuilder:default:="10Gi"
	StatefulSetSize string `json:"statefulSetSize,omitempty"`

	// 	Specify the storageClass Stateful Sets. This storage class will also
	// be used for Object Storage if MetricObjectStorage was configured for
	// the system to create the storage.
	// +optional
	// +kubebuilder:default:=gp2
	StatefulSetStorageClass string `json:"statefulSetStorageClass,omitempty"`
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
