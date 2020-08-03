// Copyright (c) 2020 Red Hat, Inc.

package v1alpha1

import (
	observatoriumv1alpha1 "github.com/observatorium/configuration/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MultiClusterObservabilitySpec defines the desired state of MultiClusterObservability
type MultiClusterObservabilitySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	// Version of the MultiClusterObservability
	// +optional
	Version string `json:"version"`

	// Repository of the MultiClusterObservability images
	// +optional
	ImageRepository string `json:"imageRepository"`

	// ImageTagSuffix of the MultiClusterObservability images
	// +optional
	ImageTagSuffix string `json:"imageTagSuffix"`

	// Pull policy of the MultiClusterObservability images
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy"`

	// Pull secret of the MultiClusterObservability images
	// +optional
	ImagePullSecret string `json:"imagePullSecret,omitempty"`

	// Spec of NodeSelector
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Spec of StorageClass
	// +optional
	StorageClass string `json:"storageClass"`

	// Spec of Observatorium
	// +optional
	Observatorium *observatoriumv1alpha1.ObservatoriumSpec `json:"observatorium"`

	// Spec of Grafana
	// +optional
	Grafana *GrafanaSpec `json:"grafana"`

	// Spec of object storage config
	// +optional
	ObjectStorageConfigSpec *ObjectStorageConfigSpec `json:"objectStorageConfigSpec,omitempty"`
}

// MultiClusterObservabilityStatus defines the observed state of MultiClusterObservability
type MultiClusterObservabilityStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// Represents the status of each deployment
	// +optional
	Deployments []DeploymentResult `json:"deployments,omitempty"`
}

// DeploymentResult defines the observed state of Deployment
type DeploymentResult struct {
	// Name of the deployment
	Name string `json:"name"`

	// The most recently observed status of the Deployment
	Status appsv1.DeploymentStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MultiClusterObservability is the Schema for the multiclusterobservability API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=multiclusterobservability,scope=Cluster,shortName=mco
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

// GrafanaSpec defines the desired state of GrafanaSpec
type GrafanaSpec struct {
	// Hostport of grafana
	// +optional
	Hostport int32 `json:"hostport"`

	// replicas of grafana
	// +optional
	Replicas int32 `json:"replicas,omitempty"`
}

// ObjectStorageConfigSpec defines the desired state of ObjectStorageConfigSpec
type ObjectStorageConfigSpec struct {
	// Type of object storage [s3 minio]
	Type string `json:"type,omitempty"`

	// Object storage configuration
	Config ObjectStorageConfig `json:"config,omitempty"`
}

// ObjectStorageConfig defines s3 object storage configuration
type ObjectStorageConfig struct {
	// Object storage bucket name
	Bucket string `json:"bucket,omitempty"`

	// Object storage server endpoint
	Endpoint string `json:"endpoint,omitempty"`

	// Configure object storage server use HTTP or HTTPs
	Insecure bool `json:"insecure,omitempty"`

	// Object storage server access key
	AccessKey string `json:"access_key,omitempty"`

	// Object storage server secret key
	SecretKey string `json:"secret_key,omitempty"`

	// Minio local PVC storage size, just for minio only, ignore it if type is s3
	// +optional
	Storage string `json:"storage,omitempty"`
}
