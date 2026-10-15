// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetAnnotation(t *testing.T) {
	tests := []struct {
		name         string
		initial      map[string]string
		key          string
		value        string
		wantModified bool
		wantVal      string
	}{
		{
			name:         "set annotation on nil map",
			initial:      nil,
			key:          "test-key",
			value:        "test-val",
			wantModified: true,
			wantVal:      "test-val",
		},
		{
			name:         "set annotation on existing map with new key",
			initial:      map[string]string{"other-key": "other-val"},
			key:          "test-key",
			value:        "test-val",
			wantModified: true,
			wantVal:      "test-val",
		},
		{
			name:         "short-circuit when annotation already has same value",
			initial:      map[string]string{"test-key": "test-val"},
			key:          "test-key",
			value:        "test-val",
			wantModified: false,
			wantVal:      "test-val",
		},
		{
			name:         "update annotation when value changes",
			initial:      map[string]string{"test-key": "old-val"},
			key:          "test-key",
			value:        "new-val",
			wantModified: true,
			wantVal:      "new-val",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.initial,
				},
			}
			modified := setAnnotation(obj, tt.key, tt.value)
			if modified != tt.wantModified {
				t.Errorf("setAnnotation() modified = %v, want %v", modified, tt.wantModified)
			}
			if got := obj.GetAnnotations()[tt.key]; got != tt.wantVal {
				t.Errorf("obj annotation[%s] = %s, want %s", tt.key, got, tt.wantVal)
			}
		})
	}
}

func TestRemoveAnnotation(t *testing.T) {
	tests := []struct {
		name         string
		initial      map[string]string
		key          string
		wantModified bool
		wantPresent  bool
	}{
		{
			name:         "short-circuit when annotations map is nil",
			initial:      nil,
			key:          "test-key",
			wantModified: false,
			wantPresent:  false,
		},
		{
			name:         "short-circuit when key does not exist",
			initial:      map[string]string{"other-key": "other-val"},
			key:          "test-key",
			wantModified: false,
			wantPresent:  false,
		},
		{
			name:         "remove existing annotation",
			initial:      map[string]string{"test-key": "test-val", "other": "val"},
			key:          "test-key",
			wantModified: true,
			wantPresent:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.initial,
				},
			}
			modified := removeAnnotation(obj, tt.key)
			if modified != tt.wantModified {
				t.Errorf("removeAnnotation() modified = %v, want %v", modified, tt.wantModified)
			}
			if _, exists := obj.GetAnnotations()[tt.key]; exists != tt.wantPresent {
				t.Errorf("obj annotation[%s] exists = %v, want %v", tt.key, exists, tt.wantPresent)
			}
		})
	}
}

func TestSetLabel(t *testing.T) {
	tests := []struct {
		name         string
		initial      map[string]string
		key          string
		value        string
		wantModified bool
		wantVal      string
	}{
		{
			name:         "set label on nil map",
			initial:      nil,
			key:          "test-key",
			value:        "test-val",
			wantModified: true,
			wantVal:      "test-val",
		},
		{
			name:         "short-circuit when label already has same value",
			initial:      map[string]string{"test-key": "test-val"},
			key:          "test-key",
			value:        "test-val",
			wantModified: false,
			wantVal:      "test-val",
		},
		{
			name:         "update label when value changes",
			initial:      map[string]string{"test-key": "old-val"},
			key:          "test-key",
			value:        "new-val",
			wantModified: true,
			wantVal:      "new-val",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: tt.initial,
				},
			}
			modified := setLabel(obj, tt.key, tt.value)
			if modified != tt.wantModified {
				t.Errorf("setLabel() modified = %v, want %v", modified, tt.wantModified)
			}
			if got := obj.GetLabels()[tt.key]; got != tt.wantVal {
				t.Errorf("obj label[%s] = %s, want %s", tt.key, got, tt.wantVal)
			}
		})
	}
}

func TestRemoveLabel(t *testing.T) {
	tests := []struct {
		name         string
		initial      map[string]string
		key          string
		wantModified bool
		wantPresent  bool
	}{
		{
			name:         "short-circuit when labels map is nil",
			initial:      nil,
			key:          "test-key",
			wantModified: false,
			wantPresent:  false,
		},
		{
			name:         "short-circuit when key does not exist",
			initial:      map[string]string{"other-key": "other-val"},
			key:          "test-key",
			wantModified: false,
			wantPresent:  false,
		},
		{
			name:         "remove existing label",
			initial:      map[string]string{"test-key": "test-val"},
			key:          "test-key",
			wantModified: true,
			wantPresent:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: tt.initial,
				},
			}
			modified := removeLabel(obj, tt.key)
			if modified != tt.wantModified {
				t.Errorf("removeLabel() modified = %v, want %v", modified, tt.wantModified)
			}
			if _, exists := obj.GetLabels()[tt.key]; exists != tt.wantPresent {
				t.Errorf("obj label[%s] exists = %v, want %v", tt.key, exists, tt.wantPresent)
			}
		})
	}
}
