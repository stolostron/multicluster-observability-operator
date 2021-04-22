// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package v1beta2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	observabilityshared "github.com/open-cluster-management/multicluster-observability-operator/api/shared"
)

// MultiClusterObservabilitySpec defines the desired state of MultiClusterObservability
type MultiClusterObservabilitySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Enable or disable the downsample.
	// The default value is true.
	// This is not recommended as querying long time ranges
	// without non-downsampled data is not efficient and useful.
	// +optional
	// +kubebuilder:default:=true
	EnableDownsampling bool `json:"enableDownsampling"`
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
	StorageConfig *StorageConfig `json:"storageConfig,omitempty"`
	// The ObservabilityAddonSpec defines the global settings for all managed
	// clusters which have observability add-on enabled.
	// +required
	ObservabilityAddonSpec *observabilityshared.ObservabilityAddonSpec `json:"observabilityAddonSpec,omitempty"`
	// Spec of resources per component
	// +optional
	Resources *ResourceConfig `json:"resources,omitempty"`
}

// ResourceConfig is the spec of resources per components
type ResourceConfig struct {
	// resources for observatorium-api
	// +optional
	// +kubebuilder:default:={requests: {cpu: "20m", memory: "128Mi"}, limits: {cpu: "1", memory: "1Gi"}}
	ObservatoriumAPI corev1.ResourceRequirements `json:"observatoriumAPI,omitempty"`
	// resources for thanos-query-frontend
	// +optional
	// +kubebuilder:default:={requests: {cpu: "100m", memory: "256Mi"}, limits: {cpu: "1", memory: "1Gi"}}
	ThanosQueryFrontend corev1.ResourceRequirements `json:"thanosQueryFrontend,omitempty"`
	// resources for thanos-query
	// +optional
	// +kubebuilder:default:={requests: {cpu: "300m", memory: "1Gi"}, limits: {cpu: "1", memory: "1Gi"}}
	ThanosQuery corev1.ResourceRequirements `json:"thanosQuery,omitempty"`
	// resources for thanos-compact
	// +optional
	// +kubebuilder:default:={requests: {cpu: "100m", memory: "512Mi"}, limits: {cpu: "1", memory: "2Gi"}}
	ThanosCompact corev1.ResourceRequirements `json:"thanosCompact,omitempty"`
	// resources for thanos-receiver
	// +optional
	// +kubebuilder:default:={requests: {cpu: "300m", memory: "512Mi"}, limits: {cpu: "1", memory: "4Gi"}}
	ThanosReceive corev1.ResourceRequirements `json:"thanosReceive,omitempty"`
	// resources for thanos-rule
	// +optional
	// +kubebuilder:default:={requests: {cpu: "50m", memory: "512Mi"}, limits: {cpu: "1", memory: "1Gi"}}
	ThanosRule corev1.ResourceRequirements `json:"thanosRule,omitempty"`
	// resources for thanos-store-shard
	// +optional
	// +kubebuilder:default:={requests: {cpu: "100m", memory: "1Gi"}, limits: {cpu: "1", memory: "2Gi"}}
	ThanosStore corev1.ResourceRequirements `json:"thanosStore,omitempty"`
	// resources for thanos-store-memcached
	// +optional
	// +kubebuilder:default:={requests: {cpu: "45m", memory: "128Mi"}, limits: {cpu: "1", memory: "1Gi"}}
	ThanosStoreMemcached corev1.ResourceRequirements `json:"thanosStoreMemcached,omitempty"`
}

// RetentionConfig is the spec of retention configurations.
type RetentionConfig struct {
	// How long to retain raw samples in a bucket.
	// It applies to --retention.resolution-raw in compact.
	// +optional
	// +kubebuilder:default:="5d"
	RetentionResolutionRaw string `json:"retentionResolutionRaw,omitempty"`
	// How long to retain samples of resolution 1 (5 minutes) in bucket.
	// It applies to --retention.resolution-5m in compact.
	// +optional
	// +kubebuilder:default:="14d"
	RetentionResolution5m string `json:"retentionResolution5m,omitempty"`
	// How long to retain samples of resolution 2 (1 hour) in bucket.
	// It applies to --retention.resolution-1h in compact.
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

// StorageConfig is the spec of object storage.
type StorageConfig struct {
	// Object store config secret for metrics
	// +required
	MetricObjectStorage *observabilityshared.PreConfiguredStorage `json:"metricObjectStorage,omitempty"`
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

// MultiClusterObservabilityStatus defines the observed state of MultiClusterObservability
type MultiClusterObservabilityStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Represents the status of each deployment
	// +optional
	Conditions []observabilityshared.Condition `json:"conditions,omitempty"`
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
