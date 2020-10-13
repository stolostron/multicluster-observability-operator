// Copyright (c) 2020 Red Hat, Inc.

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// ReplicaCount for HA support. Does not affect data stores.
	// Enabled will toggle HA support. This will provide better support in cases of failover
	// but consumes more resources. Options are: Basic and High (default).
	// +optional
	AvailabilityConfig AvailabilityType `json:"availabilityConfig,omitempty"`

	// Enable or disable the downsample.
	// The default value is false.
	// This is not recommended as querying long time ranges
	// without non-downsampled data is not efficient and useful.
	EnableDownSampling bool `json:"enableDownSampling,omitempty"`

	// Pull policy of the MultiClusterObservability images
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Pull secret of the MultiClusterObservability images
	// +optional
	ImagePullSecret string `json:"imagePullSecret,omitempty"`

	// Spec of NodeSelector
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// How long to retain raw samples in a bucket. Default is 5d
	// +optional
	RetentionResolutionRaw string `json:"retentionResolutionRaw,omitempty"`

	// How long to retain samples of resolution 1 (5 minutes) in bucket.
	// Default is 14d
	// +optional
	RetentionResolution5m string `json:"retentionResolution5m,omitempty"`

	// How long to retain samples of resolution 2 (1 hour) in bucket.
	// Default is 30d.
	// +optional
	RetentionResolution1h string `json:"retentionResolution1h,omitempty"`

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
	// The default is true
	// +optional
	EnableMetrics bool `json:"enableMetrics,omitempty"`

	// Interval for the observability addon push metrics to hub server.
	// The default is 60 seconds
	// +optional
	Interval int32 `json:"interval,omitempty"`
}

// StorageConfigObject is the spec of object storage.
type StorageConfigObject struct {
	// Object store config secret for metrics
	// +required
	MetricObjectStorage *PreConfiguredStorage `json:"metricObjectStorage,omitempty"`
	// The amount of storage applied to the Observability stateful sets, i.e.
	// Thanos store, Rule, compact and receiver.
	// The default is 10Gi
	// +optional
	StatefulSetSize string `json:"statefulSetSize,omitempty"`

	// 	Specify the storageClass Stateful Sets. This storage class will also
	// be used for Object Storage if MetricObjectStorage was configured for
	// the system to create the storage.
	// The default is gp2.
	// +optional
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
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// Represents the status of each deployment
	// +optional
	Conditions []MCOCondition `json:"conditions,omitempty"`
}

// MCOCondition defines the aggregated state of entire MultiClusterObservability CR
type MCOCondition struct {
	Type    string `json:"type,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MultiClusterObservability defines the configuration for the Observability installation on
// Hub and Managed Clusters all through this one custom resource.
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=multiclusterobservabilities,scope=Cluster,shortName=mco
type MultiClusterObservability struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MultiClusterObservabilitySpec   `json:"spec,omitempty"`
	Status MultiClusterObservabilityStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MultiClusterObservabilityList contains a list of MultiClusterObservability
type MultiClusterObservabilityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MultiClusterObservability `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MultiClusterObservability{}, &MultiClusterObservabilityList{})
}
