// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package v1beta2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	observabilityshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
)

// MultiClusterObservabilitySpec defines the desired state of MultiClusterObservability.
type MultiClusterObservabilitySpec struct {
	// Platform and UserWorkload Observability Capabilities
	// +optional
	Capabilities *CapabilitiesSpec `json:"capabilities,omitempty"`
	// Advanced configurations for observability
	// +optional
	AdvancedConfig *AdvancedConfig `json:"advanced,omitempty"`
	// Size read and write paths of your Observability instance
	// +optional
	InstanceSize TShirtSize `json:"instanceSize,omitempty"`
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

// T Shirt size class for a particular o11y resource.
// +kubebuilder:validation:Enum:={"default","minimal","small","medium","large","xlarge","2xlarge","4xlarge"}
type TShirtSize string

type LogsCollectionSpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

type PlatformCapabilitiesSpec struct {
	// +optional
	Logs LogsCollectionSpec `json:"logs,omitempty"`
}

type ClusterLogForwarderSpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

type UserWorkloadLogsSpec struct {
	ClusterLogForwarder ClusterLogForwarderSpec `json:"clusterLogForwarder,omitempty"`
}

type OpenTelemetryCollectorSpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

type InstrumentationSpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

type UserWorkloadTracesSpec struct {
	// +optional
	OpenTelemetryCollector OpenTelemetryCollectorSpec `json:"openTelemetryCollector,omitempty"`
	// +optional
	Instrumentation InstrumentationSpec `json:"instrumentation,omitempty"`
}

type UserWorkloadCapabilitiesSpec struct {
	Logs   UserWorkloadLogsSpec   `json:"logs,omitempty"`
	Traces UserWorkloadTracesSpec `json:"traces,omitempty"`
}

type CapabilitiesSpec struct {
	// +optional
	Platform *PlatformCapabilitiesSpec `json:"platform,omitempty"`
	// +optional
	UserWorkloads *UserWorkloadCapabilitiesSpec `json:"userWorkloads,omitempty"`
}

type AdvancedConfig struct {
	// CustomObservabilityHubURL overrides the endpoint used by the metrics-collector to send
	// metrics to the hub server.
	// For the metrics-collector that runs in the hub this setting has no effect.
	// +optional
	CustomObservabilityHubURL observabilityshared.URL `json:"customObservabilityHubURL,omitempty"`
	// CustomAlertmanagerHubURL overrides the alertmanager URL to send alerts from the spoke
	// to the hub server.
	// For the alertmanager that runs in the hub this setting has no effect.
	// +optional
	CustomAlertmanagerHubURL observabilityshared.URL `json:"customAlertmanagerHubURL,omitempty"`
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
	QueryFrontend *QueryFrontendSpec `json:"queryFrontend,omitempty"`
	// spec for thanos-query
	// +optional
	Query *QuerySpec `json:"query,omitempty"`
	// spec for thanos-compact
	// +optional
	Compact *CompactSpec `json:"compact,omitempty"`
	// spec for thanos-receiver
	// +optional
	Receive *ReceiveSpec `json:"receive,omitempty"`
	// spec for thanos-rule
	// +optional
	Rule *RuleSpec `json:"rule,omitempty"`
	// spec for thanos-store-shard
	// +optional
	Store *StoreSpec `json:"store,omitempty"`
	// spec for multicluster-obervability-addon
	MultiClusterObservabilityAddon *CommonSpec `json:"multiClusterObservabilityAddon,omitempty"`
}

type CommonSpec struct {
	// Compute Resources required by this component.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// Replicas for this component.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
}

// Thanos Query Spec.
type QuerySpec struct {
	// Annotations is an unstructured key value map stored with a service account
	// +optional
	ServiceAccountAnnotations map[string]string `json:"serviceAccountAnnotations,omitempty"`

	// Set to true to use the old Prometheus engine for PromQL queries.
	// +optional
	UsePrometheusEngine bool `json:"usePrometheusEngine,omitempty"`

	// WARNING: Use only with guidance from Red Hat Support. Using this feature incorrectly can
	// lead to an unrecoverable state, data loss, or both, which is not covered by Red Hat Support.
	// +optional
	Containers []corev1.Container `json:"containers,omitempty"`

	CommonSpec `json:",inline"`
}

// Thanos Receive Spec.
type ReceiveSpec struct {
	// Annotations is an unstructured key value map stored with a service account
	// +optional
	ServiceAccountAnnotations map[string]string `json:"serviceAccountAnnotations,omitempty"`

	// WARNING: Use only with guidance from Red Hat Support. Using this feature incorrectly can
	// lead to an unrecoverable state, data loss, or both, which is not covered by Red Hat Support.
	// +optional
	Containers []corev1.Container `json:"containers,omitempty"`

	CommonSpec `json:",inline"`
}

// Thanos Store Spec.
type StoreSpec struct {
	// Annotations is an unstructured key value map stored with a service account
	// +optional
	ServiceAccountAnnotations map[string]string `json:"serviceAccountAnnotations,omitempty"`

	CommonSpec `json:",inline"`

	// WARNING: Use only with guidance from Red Hat Support. Using this feature incorrectly can
	// lead to an unrecoverable state, data loss, or both, which is not covered by Red Hat Support.
	// +optional
	Containers []corev1.Container `json:"containers,omitempty"`
}

// Thanos Rule Spec.
type RuleSpec struct {
	// Evaluation interval
	// +optional
	EvalInterval string `json:"evalInterval,omitempty"`

	// Annotations is an unstructured key value map stored with a service account
	// +optional
	ServiceAccountAnnotations map[string]string `json:"serviceAccountAnnotations,omitempty"`

	// WARNING: Use only with guidance from Red Hat Support. Using this feature incorrectly can
	// lead to an unrecoverable state, data loss, or both, which is not covered by Red Hat Support.
	// +optional
	Containers []corev1.Container `json:"containers,omitempty"`

	CommonSpec `json:",inline"`
}

// Thanos Compact Spec.
type CompactSpec struct {
	// Compute Resources required by the compact.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// Annotations is an unstructured key value map stored with a service account
	// +optional
	ServiceAccountAnnotations map[string]string `json:"serviceAccountAnnotations,omitempty"`

	// WARNING: Use only with guidance from Red Hat Support. Using this feature incorrectly can
	// lead to an unrecoverable state, data loss, or both, which is not covered by Red Hat Support.
	// +optional
	Containers []corev1.Container `json:"containers,omitempty"`
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

// Thanos QueryFrontend Spec.
type QueryFrontendSpec struct {
	// WARNING: Use only with guidance from Red Hat Support. Using this feature incorrectly can
	// lead to an unrecoverable state, data loss, or both, which is not covered by Red Hat Support.
	// +optional
	Containers []corev1.Container `json:"containers,omitempty"`

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
	// WriteStorage storage config secret list for metrics
	// +optional
	WriteStorage []*observabilityshared.PreConfiguredStorage `json:"writeStorage,omitempty"`
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

// MultiClusterObservabilityStatus defines the observed state of MultiClusterObservability.
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
