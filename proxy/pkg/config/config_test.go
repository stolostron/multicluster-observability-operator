// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetManagedClusterLabelAllowListConfigmap(t *testing.T) {
	testCase := struct {
		name      string
		namespace string
		expected  error
	}{"should get managedclsuter label allowlist configmap", "ns1", nil}

	client := fake.NewSimpleClientset()
	_, err := client.CoreV1().ConfigMaps(testCase.namespace).Create(
		context.Background(),
		CreateManagedClusterLabelAllowListCM(testCase.namespace),
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Errorf("failed to create configmap: %v", err)
	}

	_, err = GetManagedClusterLabelAllowListConfigmap(context.Background(), client, testCase.namespace)
	if err != nil {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, err, testCase.expected)
	}

	testCase.name = "should not get managedcluster label allowlist configmap"
	testCase.namespace = "ns2"

	_, err = GetManagedClusterLabelAllowListConfigmap(context.Background(), client, "ns2")
	if err == nil {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, nil, err)
	}
}
