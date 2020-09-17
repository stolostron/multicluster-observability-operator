// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	"testing"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
)

func TestRemove(t *testing.T) {
	s := []string{"one", "two", "three"}
	s = Remove(s, "two")
	if len(s) != 2 {
		t.Errorf("the length of string (%v) is not the expected (2)", len(s))
	}
}

func TestGetAnnotation(t *testing.T) {
	tmpAnnotations := map[string]string{
		"repo": "1",
		"test": "2",
	}
	if GetAnnotation(tmpAnnotations, "repo") != "1" {
		t.Errorf("repo (%v) is not the expected (%v)", GetAnnotation(tmpAnnotations, "repo"), "1")
	}
	if GetAnnotation(tmpAnnotations, "failed") != "" {
		t.Errorf("failed (%v) is not the expected (%v)", GetAnnotation(tmpAnnotations, "repo"), "")
	}
}

func TestGetReplicaCount(t *testing.T) {
	var replicas1 int32 = 1
	var replicas2 int32 = 2
	var replicas3 int32 = 3
	caseList := []struct {
		availabilityType mcov1beta1.AvailabilityType
		resourceType     string
		name             string
		expected         *int32
	}{
		{
			availabilityType: mcov1beta1.HABasic,
			name:             "Have 1 instance",
			resourceType:     "",
			expected:         &replicas1,
		},
		{
			availabilityType: mcov1beta1.HAHigh,
			name:             "Have 2 instances",
			resourceType:     "Deployments",
			expected:         &replicas2,
		},
		{
			availabilityType: mcov1beta1.HAHigh,
			name:             "Have 3 instances",
			resourceType:     "StatefulSet",
			expected:         &replicas3,
		},
		{
			availabilityType: mcov1beta1.HAHigh,
			name:             "Have 2 instances",
			resourceType:     "",
			expected:         &replicas2,
		},
	}
	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			output := GetReplicaCount(c.availabilityType, c.resourceType)
			if *output != *c.expected {
				t.Errorf("case (%v) output (%v) is not the expected (%v)", c.name, output, c.expected)
			}
		})
	}
}
