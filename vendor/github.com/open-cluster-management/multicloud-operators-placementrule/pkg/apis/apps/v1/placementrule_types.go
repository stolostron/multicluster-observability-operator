// Copyright 2019 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
)

const (
	// SchedulerNameDefault tells using default scheduler (mcm)
	SchedulerNameDefault = "default"
	// SchedulerNameMCM tells using mcm as scheduler
	SchedulerNameMCM = "mcm"

	// UserIdentityAnnotation is user identity annotation
	UserIdentityAnnotation = "mcm.ibm.com/user-identity"

	// UserGroupAnnotation is user group annotation
	UserGroupAnnotation = "mcm.ibm.com/user-group"
)

// +k8s:deepcopy-gen:nonpointer-interfaces=true
// Placement field to be referenced in specs, align with Fedv2, add placementref
type Placement struct {
	GenericPlacementFields `json:",inline"`
	PlacementRef           *corev1.ObjectReference `json:"placementRef,omitempty"`
	Local                  *bool                   `json:"local,omitempty"`
}

// ClusterConditionFilter defines filter to filter cluster condition
type ClusterConditionFilter struct {
	// +optional
	Type clusterv1alpha1.ClusterConditionType `json:"type,omitempty"`
	// +optional
	Status corev1.ConditionStatus `json:"status,omitempty"`
}

// ResourceType defines types can be sorted
type ResourceType string

// These are valid conditions of a cluster.
const (
	ResourceTypeNone   ResourceType = ""
	ResourceTypeCPU    ResourceType = "cpu"
	ResourceTypeMemory ResourceType = "memory"
)

// SelectionOrder is the type for Nodes
type SelectionOrder string

// These are valid conditions of a cluster.
const (
	SelectionOrderNone SelectionOrder = ""
	SelectionOrderDesc SelectionOrder = "desc"
	SelectionOrderAsce SelectionOrder = "asc"
)

// ResourceHint is used to sort the output
type ResourceHint struct {
	Type  ResourceType   `json:"type,omitempty"`
	Order SelectionOrder `json:"order,omitempty"`
}

// GenericClusterReference - in alignment with kubefed
type GenericClusterReference struct {
	Name string `json:"name"`
}

// GenericPlacementFields - in alignment with kubefed
type GenericPlacementFields struct {
	Clusters        []GenericClusterReference `json:"clusters,omitempty"`
	ClusterSelector *metav1.LabelSelector     `json:"clusterSelector,omitempty"`
}

// PlacementRuleSpec defines the desired state of PlacementRule
type PlacementRuleSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// +optional
	// schedulerName, default to use mcm controller
	SchedulerName string `json:"schedulerName,omitempty"`
	// +optional
	// number of replicas Application wants to
	ClusterReplicas *int32 `json:"clusterReplicas,omitempty"`
	// +optional
	GenericPlacementFields `json:",inline"`
	// +optional
	ClusterConditions []ClusterConditionFilter `json:"clusterConditions,omitempty"`
	// +optional
	// Select Resource
	ResourceHint *ResourceHint `json:"resourceHint,omitempty"`
	// +optional
	// Set Policy Filters
	Policies []corev1.ObjectReference `json:"policies,omitempty"`
}

// PlacementDecision defines the decision made by controller
type PlacementDecision struct {
	ClusterName      string `json:"clusterName,omitempty"`
	ClusterNamespace string `json:"clusterNamespace,omitempty"`
}

// PlacementRuleStatus defines the observed state of PlacementRule
type PlacementRuleStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Decisions []PlacementDecision `json:"decisions,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PlacementRule is the Schema for the placementrules API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type PlacementRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlacementRuleSpec   `json:"spec,omitempty"`
	Status PlacementRuleStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PlacementRuleList contains a list of PlacementRule
type PlacementRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PlacementRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PlacementRule{}, &PlacementRuleList{})
}
