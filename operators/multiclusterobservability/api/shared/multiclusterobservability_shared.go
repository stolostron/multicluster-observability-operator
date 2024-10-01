// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

// Package shared contains shared API Schema definitions for the observability API group
// +kubebuilder:object:generate=true
// +groupName=observability.open-cluster-management.io

package shared

import (
	"net/url"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// URL is kubebuilder type that validates the containing string is an HTTPS URL.
// +kubebuilder:validation:Pattern=`^https:\/\/`
// +kubebuilder:validation:MaxLength=2083
type URL string

// Validate validates the underlying URL.
func (u URL) Validate() error {
	_, err := url.Parse(string(u))
	return err
}

func (u URL) URL() (*url.URL, error) {
	return url.Parse(string(u))
}

// ObservabilityAddonSpec is the spec of observability addon.
type ObservabilityAddonSpec struct {
	// EnableMetrics indicates the observability addon push metrics to hub server.
	// +optional
	// +kubebuilder:default:=true
	EnableMetrics bool `json:"enableMetrics"`

	// Interval for the observability addon push metrics to hub server.
	// +optional
	// +kubebuilder:default:=300
	// +kubebuilder:validation:Minimum=15
	// +kubebuilder:validation:Maximum=3600
	Interval int32 `json:"interval,omitempty"`

	// ScrapeSizeLimitBytes is the max size in bytes for a single metrics scrape from in-cluster Prometheus.
	// Default is 1 GiB.
	// +kubebuilder:default:=1073741824
	ScrapeSizeLimitBytes int `json:"scrapeSizeLimitBytes,omitempty"`

	// Workers is the number of workers in metrics-collector that work in parallel to
	// push metrics to hub server. If set to > 1, metrics-collector will shard
	// /federate calls to Prometheus, based on matcher rules provided by allowlist.
	// Ensure that number of matchers exceeds number of workers.
	// +optional
	// +kubebuilder:default:=1
	// +kubebuilder:validation:Minimum=1
	Workers int32 `json:"workers,omitempty"`

	// Resource requirement for metrics-collector
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

type PreConfiguredStorage struct {
	// The key of the secret to select from. Must be a valid secret key.
	// Refer to https://thanos.io/tip/thanos/storage.md/#configuring-access-to-object-storage for a valid content of key.
	// +required
	Key string `json:"key"`
	// Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// +required
	Name string `json:"name"`
	// TLS secret contains the custom certificate for the object store
	// +optional
	TLSSecretName string `json:"tlsSecretName,omitempty"`
	// TLS secret mount path for the custom certificate for the object store
	// +optional
	TLSSecretMountPath string `json:"tlsSecretMountPath,omitempty"`
	// serviceAccountProjection indicates whether mount service account token to thanos pods. Default is false.
	// +optional
	ServiceAccountProjection bool `json:"serviceAccountProjection,omitempty"`
}

// Condition is from metav1.Condition.
// Cannot use it directly because the upgrade issue.
// Have to mark LastTransitionTime and Status as optional.
type Condition struct {
	// type of condition in CamelCase or in foo.example.com/CamelCase.
	// ---
	// Many .condition.type values are consistent across resources like Available, but because arbitrary conditions can be
	// useful (see .node.status.conditions), the ability to deconflict is important.
	// The regex it matches is (dns1123SubdomainFmt/)?(qualifiedNameFmt)
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
	// +kubebuilder:validation:MaxLength=316
	Type string `json:"type" protobuf:"bytes,1,opt,name=type"`
	// status of the condition, one of True, False, Unknown.
	// +optional
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=True;False;Unknown
	Status metav1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status"`
	// observedGeneration represents the .metadata.generation that the condition was set based upon.
	// For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9,
	// the condition is out of date
	// with respect to the current state of the instance.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,3,opt,name=observedGeneration"`
	// lastTransitionTime is the last time the condition transitioned from one status to another.
	// This should be when the underlying condition changed.  If that is not known, then using the time when the API
	// field changed is acceptable.
	// +optional
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=date-time
	LastTransitionTime metav1.Time `json:"lastTransitionTime" protobuf:"bytes,4,opt,name=lastTransitionTime"`
	// reason contains a programmatic identifier indicating the reason for the condition's last transition.
	// Producers of specific condition types may define expected values and meanings for this field,
	// and whether the values are considered a guaranteed API.
	// The value should be a CamelCase string.
	// This field may not be empty.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=1024
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$`
	Reason string `json:"reason" protobuf:"bytes,5,opt,name=reason"`
	// message is a human readable message indicating details about the transition.
	// This may be an empty string.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=32768
	Message string `json:"message" protobuf:"bytes,6,opt,name=message"`
}
