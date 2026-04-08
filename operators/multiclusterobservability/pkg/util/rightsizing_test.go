// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"testing"

	obv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsRightSizingDelegated(t *testing.T) {
	tests := []struct {
		name     string
		cr       *obv1beta2.MultiClusterObservability
		expected bool
	}{
		{
			name:     "no annotations",
			cr:       &obv1beta2.MultiClusterObservability{},
			expected: false,
		},
		{
			name: "other annotation only",
			cr: &obv1beta2.MultiClusterObservability{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"other": "value"},
				},
			},
			expected: false,
		},
		{
			name: "annotation present",
			cr: &obv1beta2.MultiClusterObservability{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{RightSizingCapableAnnotation: "true"},
				},
			},
			expected: true,
		},
		{
			name: "annotation present with empty value",
			cr: &obv1beta2.MultiClusterObservability{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{RightSizingCapableAnnotation: ""},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRightSizingDelegated(tt.cr); got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}
