// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"testing"
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
	var replicas2 int32 = 2
	var replicas3 int32 = 3
	caseList := []struct {
		resourceType string
		name         string
		expected     *int32
	}{
		{
			name:         "Have 2 instances",
			resourceType: "Deployments",
			expected:     &replicas2,
		},
		{
			name:         "Have 3 instances",
			resourceType: "StatefulSet",
			expected:     &replicas3,
		},
		{
			name:         "Have 2 instances",
			resourceType: "",
			expected:     &replicas2,
		},
	}
	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			output := GetReplicaCount(c.resourceType)
			if *output != *c.expected {
				t.Errorf("case (%v) output (%v) is not the expected (%v)", c.name, output, c.expected)
			}
		})
	}
}
