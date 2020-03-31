package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MultiClusterMonitoringSpec defines the desired state of MultiClusterMonitoring
type MultiClusterMonitoringSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	// Version of the MultiClusterMonitor
	Version string `json:"version"`

	// Repository of the MultiClusterMonitor images
	ImageRepository string `json:"imageRepository"`

	// ImageTagSuffix of the MultiClusterMonitor images
	ImageTagSuffix string `json:"imageTagSuffix"`

	// Pull policy of the MultiClusterMonitor images
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy"`

	// Pull secret of the MultiClusterMonitor images
	// +optional
	ImagePullSecret string `json:"imagePullSecret,omitempty"`

	// Spec of NodeSelector
	// +optional
	NodeSelector *NodeSelector `json:"nodeSelector,omitempty"`

	// Spec of Observatorium
	Observatorium ObservatoriumSpec `json:"observatorium"`

	// Spec of Grafana
	Grafana GrafanaSpec `json:"grafana"`
}

// MultiClusterMonitoringStatus defines the observed state of MultiClusterMonitoring
type MultiClusterMonitoringStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MultiClusterMonitoring is the Schema for the multiclustermonitorings API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=multiclustermonitorings,scope=Namespaced
type MultiClusterMonitoring struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MultiClusterMonitoringSpec   `json:"spec,omitempty"`
	Status MultiClusterMonitoringStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MultiClusterMonitoringList contains a list of MultiClusterMonitoring
type MultiClusterMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MultiClusterMonitoring `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MultiClusterMonitoring{}, &MultiClusterMonitoringList{})
}

// NodeSelector defines the desired state of NodeSelector
type NodeSelector struct {
	// Spec of OS
	// +optional
	OS string `json:"os,omitempty"`

	// Spec of CustomLabelSelector
	// +optional
	CustomLabelSelector string `json:"customLabelSelector,omitempty"`

	// Spec of CustomLabelValue
	// +optional
	CustomLabelValue string `json:"customLabelValue,omitempty"`
}

// ObservatoriumSpec defines the desired state of Observatorium
type ObservatoriumSpec struct {
	// Enabled for enable observatorium
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// StorageClass for Observatorium component
	StorageClass string `json:"storageClass,omitempty"`

	// CompactVolumeSize for compact volume size (ex. 50Gi)
	CompactVolumeSize string `json:"compactVolumeSize,omitempty"`

	// ReceiversVolumeSize for receivers volume size (ex. 50Gi)
	ReceiversVolumeSize string `json:"receiversVolumeSize,omitempty"`

	// RuleVolumeSize for rule volume size (ex. 50Gi)
	RuleVolumeSize string `json:"ruleVolumeSize,omitempty"`

	// StoreVolumeSize for store volume size (ex. 50Gi)
	StoreVolumeSize string `json:"storeVolumeSize,omitempty"`
}

// GrafanaSpec defines the desired state of Grafana
type GrafanaSpec struct {
	// Enabled for enable grafana
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}
