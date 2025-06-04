// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

const (
	mockRsNamespace     = "open-cluster-management-observability"
	mockConfigMapNS     = "open-cluster-management-global-set"
	mockConfigMapKey    = "config.yaml"
	mockRsConfigMapName = "rs-namespace-config"
)

func TestCleanupRSNamespaceResources_WithBindingUpdated(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = policyv1.AddToScheme(scheme)
	_ = clusterv1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&policyv1.PlacementBinding{ObjectMeta: metav1.ObjectMeta{Name: rsPlacementBindingName, Namespace: mockRsNamespace}},
		&clusterv1beta1.Placement{ObjectMeta: metav1.ObjectMeta{Name: rsPlacementName, Namespace: mockRsNamespace}},
		&policyv1.Policy{ObjectMeta: metav1.ObjectMeta{Name: rsPrometheusRulePolicyName, Namespace: mockRsNamespace}},
	).Build()

	cleanupRSNamespaceResources(context.TODO(), k8sClient, mockRsNamespace, true)

	for _, obj := range []client.Object{
		&policyv1.PlacementBinding{ObjectMeta: metav1.ObjectMeta{Name: rsPlacementBindingName, Namespace: mockRsNamespace}},
		&clusterv1beta1.Placement{ObjectMeta: metav1.ObjectMeta{Name: rsPlacementName, Namespace: mockRsNamespace}},
		&policyv1.Policy{ObjectMeta: metav1.ObjectMeta{Name: rsPrometheusRulePolicyName, Namespace: mockRsNamespace}},
	} {
		err := k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
		assert.Error(t, err)
	}
}

func TestCleanupRSNamespaceResources_WithBindingNotUpdated(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = policyv1.AddToScheme(scheme)
	_ = clusterv1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	configMapNS := config.GetDefaultNamespace()

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&policyv1.PlacementBinding{ObjectMeta: metav1.ObjectMeta{Name: rsPlacementBindingName, Namespace: mockRsNamespace}},
		&clusterv1beta1.Placement{ObjectMeta: metav1.ObjectMeta{Name: rsPlacementName, Namespace: mockRsNamespace}},
		&policyv1.Policy{ObjectMeta: metav1.ObjectMeta{Name: rsPrometheusRulePolicyName, Namespace: mockRsNamespace}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: rsConfigMapName, Namespace: configMapNS}},
	).Build()

	cleanupRSNamespaceResources(context.TODO(), k8sClient, mockRsNamespace, false)

	for _, obj := range []client.Object{
		&policyv1.PlacementBinding{ObjectMeta: metav1.ObjectMeta{Name: rsPlacementBindingName, Namespace: mockRsNamespace}},
		&clusterv1beta1.Placement{ObjectMeta: metav1.ObjectMeta{Name: rsPlacementName, Namespace: mockRsNamespace}},
		&policyv1.Policy{ObjectMeta: metav1.ObjectMeta{Name: rsPrometheusRulePolicyName, Namespace: mockRsNamespace}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: rsConfigMapName, Namespace: configMapNS}},
	} {
		err := k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
		assert.Error(t, err)
	}
}
