package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MultiClusterMonitoringSpec defines the desired state of MultiClusterMonitoring
type MultiClusterMonitoringSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
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
