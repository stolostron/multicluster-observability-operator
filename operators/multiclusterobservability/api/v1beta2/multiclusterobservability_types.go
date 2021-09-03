// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package v1beta2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	observabilityshared "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
)

// MultiClusterObservabilitySpec defines the desired state of MultiClusterObservability
type MultiClusterObservabilitySpec struct {
	// Advanced configurations for observability
	// +optional
	AdvancedConfig *AdvancedConfig `json:"advanced,omitempty"`
	// Enable or disable the downsample.
	// +optional
	// +kubebuilder:default:=true
	EnableDownsampling bool `json:"enableDownsampling"`
	// Pull policy of the MultiClusterObservability images
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// Pull secret of the MultiClusterObservability images
	// +optional
	ImagePullSecret string `json:"imagePullSecret,omitempty"`
	// Spec of NodeSelector
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations causes all components to tolerate any taints.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// Specifies the storage to be used by Observability
	// +required
	StorageConfig *StorageConfig `json:"storageConfig"`
	// The ObservabilityAddonSpec defines the global settings for all managed
	// clusters which have observability add-on enabled.
	// +required
	ObservabilityAddonSpec *observabilityshared.ObservabilityAddonSpec `json:"observabilityAddonSpec"`
}

type AdvancedConfig struct {
	// The spec of the data retention configurations
	// +optional
	RetentionConfig *RetentionConfig `json:"retentionConfig,omitempty"`
	// The spec of rbac-query-proxy
	// +optional
	RBACQueryProxy *CommonSpec `json:"rbacQueryProxy,omitempty"`
	// The spec of grafana
	// +optional
	Grafana *CommonSpec `json:"grafana,omitempty"`
	// The spec of alertmanager
	// +optional
	Alertmanager *CommonSpec `json:"alertmanager,omitempty"`
	// Specifies the store memcached
	// +optional
	StoreMemcached *CacheConfig `json:"storeMemcached,omitempty"`
	// Specifies the store memcached
	// +optional
	QueryFrontendMemcached *CacheConfig `json:"queryFrontendMemcached,omitempty"`
	// Spec of observatorium api
	// +optional
	ObservatoriumAPI *CommonSpec `json:"observatoriumAPI,omitempty"`
	// spec for thanos-query-frontend
	// +optional
	QueryFrontend *CommonSpec `json:"queryFrontend,omitempty"`
	// spec for thanos-query
	// +optional
	Query *CommonSpec `json:"query,omitempty"`
	// spec for thanos-compact
	// +optional
	Compact *CompactSpec `json:"compact,omitempty"`
	// spec for thanos-receiver
	// +optional
	Receive *CommonSpec `json:"receive,omitempty"`
	// spec for thanos-rule
	// +optional
	Rule *CommonSpec `json:"rule,omitempty"`
	// spec for thanos-store-shard
	// +optional
	Store *CommonSpec `json:"store,omitempty"`
}

type CommonSpec struct {
	// Compute Resources required by this component.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// Replicas for this component.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
}

// Thanos Compact Spec
type CompactSpec struct {
	// Compute Resources required by the compact.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// CacheConfig is the spec of memcached.
type CacheConfig struct {
	// Memory limit of Memcached in megabytes.
	// +optional
	MemoryLimitMB *int32 `json:"memoryLimitMb,omitempty"`
	// Max item size of Memcached (default: 1m, min: 1k, max: 1024m).
	// +optional
	MaxItemSize string `json:"maxItemSize,omitempty"`
	// Max simultaneous connections of Memcached.
	// +optional
	ConnectionLimit *int32 `json:"connectionLimit,omitempty"`

	CommonSpec `json:",inline"`
}

// RetentionConfig is the spec of retention configurations.
type RetentionConfig struct {
	// How long to retain raw samples in a bucket.
	// It applies to --retention.resolution-raw in compact.
	// +optional
	RetentionResolutionRaw string `json:"retentionResolutionRaw,omitempty"`
	// How long to retain samples of resolution 1 (5 minutes) in bucket.
	// It applies to --retention.resolution-5m in compact.
	// +optional
	RetentionResolution5m string `json:"retentionResolution5m,omitempty"`
	// How long to retain samples of resolution 2 (1 hour) in bucket.
	// It applies to --retention.resolution-1h in compact.
	// +optional
	RetentionResolution1h string `json:"retentionResolution1h,omitempty"`
	// How long to retain raw samples in a local disk. It applies to rule/receive:
	// --tsdb.retention in receive
	// --tsdb.retention in rule
	// +optional
	RetentionInLocal string `json:"retentionInLocal,omitempty"`
	// configure --delete-delay in compact
	// Time before a block marked for deletion is deleted from bucket.
	// +optional
	DeleteDelay string `json:"deleteDelay,omitempty"`
	// configure --tsdb.block-duration in rule (Block duration for TSDB block)
	// +optional
	BlockDuration string `json:"blockDuration,omitempty"`
}

// StorageConfig is the spec of object storage.
type StorageConfig struct {
	// Object store config secret for metrics
	// +required
	MetricObjectStorage *observabilityshared.PreConfiguredStorage `json:"metricObjectStorage"`
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
// +operator-sdk:csv:customresourcedefinitions:displayName="MultiClusterObservability"
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
