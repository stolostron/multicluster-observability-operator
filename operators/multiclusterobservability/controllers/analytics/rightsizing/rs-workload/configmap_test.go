// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsworkload

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testNamespace = "open-cluster-management-observability"

func TestEnsureRSWorkloadConfigMapExists_CreatesIfNotExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	err := EnsureRSWorkloadConfigMapExists(context.TODO(), client)
	require.NoError(t, err)

	fetched := &corev1.ConfigMap{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: ConfigMapName, Namespace: testNamespace}, fetched)
	require.NoError(t, err)
	assert.Contains(t, fetched.Data, "prometheusRuleConfig")
	assert.Contains(t, fetched.Data, "placementConfiguration")
}
