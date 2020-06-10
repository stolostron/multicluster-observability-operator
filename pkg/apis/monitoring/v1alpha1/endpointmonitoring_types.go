package v1alpha1

import (
	monv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EndpointMonitoringSpec defines the desired state of EndpointMonitoring
type EndpointMonitoringSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	GlobalConfig         GlobalConfigSpec       `json:"global"`
	MetricsCollectorList []MetricsCollectorSpec `json:"metricsCollectors"`
}

// GlobalConfigSpec defines the global configuration for metrics push in managed cluster
type GlobalConfigSpec struct {
	SeverURL  string           `json:"serverUrl"`
	TLSConfig *monv1.TLSConfig `json:"tlsConfig,omitempty"`
}

// MetricsCollectorSpec defines the configuration for one metrics collector
type MetricsCollectorSpec struct {
	Enable         bool                  `json:"enable"`
	Type           string                `json:"type"`
	RelabelConfigs []monv1.RelabelConfig `json:"relabelConfigs,omitempty"`
}

// EndpointMonitoringStatus defines the observed state of EndpointMonitoring
type EndpointMonitoringStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EndpointMonitoring is the Schema for the endpointmonitorings API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=endpointmonitorings,scope=Namespaced
type EndpointMonitoring struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointMonitoringSpec   `json:"spec,omitempty"`
	Status EndpointMonitoringStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EndpointMonitoringList contains a list of EndpointMonitoring
type EndpointMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EndpointMonitoring `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EndpointMonitoring{}, &EndpointMonitoringList{})
}
