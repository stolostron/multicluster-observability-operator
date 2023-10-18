// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	observabilityshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
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
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Conditions []StatusCondition `json:"conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ObservabilityAddon is the Schema for the observabilityaddon API
// +kubebuilder:resource:path=observabilityaddons,scope=Namespaced,shortName=oba
// +operator-sdk:csv:customresourcedefinitions:displayName="ObservabilityAddon"
type ObservabilityAddon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   observabilityshared.ObservabilityAddonSpec `json:"spec,omitempty"`
	Status ObservabilityAddonStatus                   `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ObservabilityAddonList contains a list of ObservabilityAddon
type ObservabilityAddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ObservabilityAddon `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ObservabilityAddon{}, &ObservabilityAddonList{})
}
