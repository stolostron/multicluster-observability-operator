// Copyright (c) 2020 Red Hat, Inc.

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// StatusCondition contains condition information for an observability addon
type StatusCondition struct {
	Type               string                 `json:"type"`
	Status             metav1.ConditionStatus `json:"status"`
	LastTransitionTime metav1.Time            `json:"lastTransitionTime"`
	Reason             string                 `json:"reason"`
	Message            string                 `json:"message"`
}

// ObservabilityAddonStatus defines the observed state of ObservabilityAddon
type ObservabilityAddonStatus struct {
	Conditions []StatusCondition `json:"conditions"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ObservabilityAddon is the Schema for the observabilityaddon API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=observabilityaddons,scope=Namespaced,shortName=oba
type ObservabilityAddon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ObservabilityAddonSpec   `json:"spec,omitempty"`
	Status ObservabilityAddonStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ObservabilityAddonList contains a list of ObservabilityAddon
type ObservabilityAddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ObservabilityAddon `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ObservabilityAddon{}, &ObservabilityAddonList{})
}
