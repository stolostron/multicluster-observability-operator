// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package v1beta1

import (
	observabilityshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AvailabilityType ...
type AvailabilityType string

const (
	// HABasic stands up most app subscriptions with a replicaCount of 1.
	HABasic AvailabilityType = "Basic"
	// HAHigh stands up most app subscriptions with a replicaCount of 2.
	HAHigh AvailabilityType = "High"
)

// Configuration for the hub-side Observability stack.
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
	// +optional
	// +kubebuilder:default:=false
	EnableDownSampling bool `json:"enableDownSampling"`

	// Pull policy of the MultiClusterObservability images.
	// One of: Always, Never, IfNotPresent. Defaults to IfNotPresent.
	// +optional
	// +kubebuilder:default:=IfNotPresent
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Name of an existing Secret of type kubernetes.io/dockerconfigjson in the same namespace
	// used for pulling MultiClusterObservability images.
	// +optional
	// +kubebuilder:default:=multiclusterhub-operator-pull-secret
	ImagePullSecret string `json:"imagePullSecret,omitempty"`

	// Node labels used to schedule all Observability workloads.
	// Pods are only placed on nodes matching all specified labels.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations causes all components to tolerate any taints.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// How long to retain raw samples in a bucket.
	// Format: duration string, e.g. '5d'. Raw data has the highest storage cost per day retained.
	// +optional
	// +kubebuilder:default:="5d"
	RetentionResolutionRaw string `json:"retentionResolutionRaw,omitempty"`

	// How long to retain 5-minute downsampled (resolution 1) samples in object storage.
	// Format: duration string, e.g. '14d'.
	// +optional
	// +kubebuilder:default:="14d"
	RetentionResolution5m string `json:"retentionResolution5m,omitempty"`

	// How long to retain 1-hour downsampled (resolution 2) samples in object storage.
	// Format: duration string, e.g. '365d'.
	// +optional
	// +kubebuilder:default:="30d"
	RetentionResolution1h string `json:"retentionResolution1h,omitempty"`

	// S3 storage configuration for Observability StatefulSets and object storage.
	// +required
	StorageConfig *StorageConfigObject `json:"storageConfigObject,omitempty"`

	// The ObservabilityAddonSpec defines the global settings for all managed
	// clusters which have observability add-on enabled.
	// +optional
	ObservabilityAddonSpec *observabilityshared.ObservabilityAddonSpec `json:"observabilityAddonSpec,omitempty"`
}

// StorageConfigObject is the spec of object storage.
type StorageConfigObject struct {
	// Reference to a Secret containing the Thanos object storage configuration.
	// The secret must use the Thanos storage config format (YAML).
	// See spec.storageConfigObject.metricObjectStorage.key.
	// +required
	MetricObjectStorage *observabilityshared.PreConfiguredStorage `json:"metricObjectStorage,omitempty"`

	// The amount of storage applied to the Observability stateful sets, i.e.
	// Thanos store, Rule, compact and receiver.
	// +optional
	// +kubebuilder:default:="10Gi"
	StatefulSetSize string `json:"statefulSetSize,omitempty"`

	// StorageClass used by Observability StatefulSets (Thanos store, ruler, compactor, receiver)
	// and, when no object storage is configured, the object storage PVC.
	// +optional
	// +kubebuilder:default:=gp2
	StatefulSetStorageClass string `json:"statefulSetStorageClass,omitempty"`
}

// Observed state of the MultiClusterObservability installation, including component health and addon status.
type MultiClusterObservabilityStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions describing the current state of the MultiClusterObservability installation.
	// Condition types include: Ready, Installing, Failed, MetricsDisabled, MultiClusterObservabilityAddonDegraded.
	// +optional
	Conditions []observabilityshared.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

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
