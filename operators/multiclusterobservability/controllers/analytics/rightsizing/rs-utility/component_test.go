// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"context"
	"testing"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func setupRSUtilityScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, clusterv1beta1.AddToScheme(scheme))
	require.NoError(t, policyv1.AddToScheme(scheme))
	return scheme
}

func rsLabeledMeta(name, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
		Labels:    map[string]string{RSManagedByLabel: RSManagedByValue},
	}
}

func TestGetComponentConfig(t *testing.T) {
	tests := []struct {
		name          string
		mco           *mcov1beta2.MultiClusterObservability
		componentType ComponentType
		wantEnabled   bool
		wantBinding   string
		wantErr       bool
	}{
		{
			name: "namespace enabled with binding",
			mco: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						Platform: &mcov1beta2.PlatformCapabilitiesSpec{
							Analytics: mcov1beta2.PlatformAnalyticsSpec{
								NamespaceRightSizingRecommendation: mcov1beta2.PlatformRightSizingRecommendationSpec{
									Enabled:          true,
									NamespaceBinding: "custom-ns",
								},
							},
						},
					},
				},
			},
			componentType: ComponentTypeNamespace,
			wantEnabled:   true,
			wantBinding:   "custom-ns",
		},
		{
			name: "virtualization disabled no binding",
			mco: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						Platform: &mcov1beta2.PlatformCapabilitiesSpec{},
					},
				},
			},
			componentType: ComponentTypeVirtualization,
			wantEnabled:   false,
			wantBinding:   "",
		},
		{
			name:          "nil capabilities returns false",
			mco:           &mcov1beta2.MultiClusterObservability{},
			componentType: ComponentTypeNamespace,
			wantEnabled:   false,
			wantBinding:   "",
		},
		{
			name: "unknown component type returns error",
			mco: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						Platform: &mcov1beta2.PlatformCapabilitiesSpec{},
					},
				},
			},
			componentType: ComponentType("unknown"),
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled, binding, err := GetComponentConfig(tt.mco, tt.componentType)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantEnabled, enabled)
			require.Equal(t, tt.wantBinding, binding)
		})
	}
}

func TestCleanupLegacyPolicyResourcesByName(t *testing.T) {
	scheme := setupRSUtilityScheme(t)

	nsNS := "ns-namespace"
	virtNS := "virt-namespace"

	// Pre-create the well-known legacy resources in both namespaces
	objects := []client.Object{
		&policyv1.Policy{ObjectMeta: metav1.ObjectMeta{Name: legacyNSPolicyName, Namespace: nsNS}},
		&policyv1.PlacementBinding{ObjectMeta: metav1.ObjectMeta{Name: legacyNSPlacementBindingName, Namespace: nsNS}},
		&clusterv1beta1.Placement{ObjectMeta: metav1.ObjectMeta{Name: legacyNSPlacementName, Namespace: nsNS}},
		&policyv1.Policy{ObjectMeta: metav1.ObjectMeta{Name: legacyVirtPolicyName, Namespace: virtNS}},
		&policyv1.PlacementBinding{ObjectMeta: metav1.ObjectMeta{Name: legacyVirtPlacementBindingName, Namespace: virtNS}},
		&clusterv1beta1.Placement{ObjectMeta: metav1.ObjectMeta{Name: legacyVirtPlacementName, Namespace: virtNS}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

	require.NoError(t, CleanupLegacyPolicyResourcesByName(context.TODO(), c, nsNS, virtNS))

	// All 6 resources must be deleted
	for _, obj := range objects {
		key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
		err := c.Get(context.TODO(), key, obj.DeepCopyObject().(client.Object))
		require.True(t, apierrors.IsNotFound(err), "%s %s/%s should be deleted", obj.GetObjectKind().GroupVersionKind().Kind, key.Namespace, key.Name)
	}
}

func TestCleanupLegacyPolicyResourcesByName_NotFoundIsIgnored(t *testing.T) {
	scheme := setupRSUtilityScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	// No resources exist — should succeed without error
	require.NoError(t, CleanupLegacyPolicyResourcesByName(context.TODO(), c, "ns1", "ns2"))
}

// TestCleanupLegacyPolicyResourcesByLabel_PreservesConfigMap verifies that the migration
// path deletes Policy/PlacementBinding/Placement resources but keeps ConfigMaps intact.
// ConfigMaps are preserved because MCOA reuses them for per-cluster configuration.
func TestCleanupLegacyPolicyResourcesByLabel_PreservesConfigMap(t *testing.T) {
	scheme := setupRSUtilityScheme(t)

	ns := DefaultNamespace
	policy := &policyv1.Policy{ObjectMeta: rsLabeledMeta("rs-prom-rules-policy", ns)}
	pb := &policyv1.PlacementBinding{ObjectMeta: rsLabeledMeta("rs-policyset-binding", ns)}
	placement := &clusterv1beta1.Placement{ObjectMeta: rsLabeledMeta("rs-placement", "")}
	cm := &corev1.ConfigMap{ObjectMeta: rsLabeledMeta("rs-namespace-config", ns)}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(policy, pb, placement, cm).Build()

	require.NoError(t, CleanupLegacyPolicyResourcesByLabel(context.TODO(), c))

	// Policy, PlacementBinding, Placement must be deleted.
	err := c.Get(context.TODO(), types.NamespacedName{Name: policy.Name, Namespace: ns}, &policyv1.Policy{})
	require.True(t, apierrors.IsNotFound(err), "Policy must be deleted by legacy cleanup")

	err = c.Get(context.TODO(), types.NamespacedName{Name: pb.Name, Namespace: ns}, &policyv1.PlacementBinding{})
	require.True(t, apierrors.IsNotFound(err), "PlacementBinding must be deleted by legacy cleanup")

	err = c.Get(context.TODO(), types.NamespacedName{Name: placement.Name}, &clusterv1beta1.Placement{})
	require.True(t, apierrors.IsNotFound(err), "Placement must be deleted by legacy cleanup")

	// ConfigMap must be preserved — MCOA reuses it.
	err = c.Get(context.TODO(), types.NamespacedName{Name: cm.Name, Namespace: ns}, &corev1.ConfigMap{})
	require.NoError(t, err, "ConfigMap must NOT be deleted by legacy cleanup (MCOA reuses it)")
}

// TestCleanupRSResourcesByLabel_DeletesConfigMap verifies that full MCO deletion cleanup
// removes ConfigMaps in addition to Policy/PlacementBinding/Placement resources.
func TestCleanupRSResourcesByLabel_DeletesConfigMap(t *testing.T) {
	scheme := setupRSUtilityScheme(t)

	ns := DefaultNamespace
	policy := &policyv1.Policy{ObjectMeta: rsLabeledMeta("rs-prom-rules-policy", ns)}
	placement := &clusterv1beta1.Placement{ObjectMeta: rsLabeledMeta("rs-placement", "")}
	cm := &corev1.ConfigMap{ObjectMeta: rsLabeledMeta("rs-namespace-config", ns)}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(policy, placement, cm).Build()

	require.NoError(t, CleanupRSResourcesByLabel(context.TODO(), c))

	// All resources including ConfigMap must be deleted.
	err := c.Get(context.TODO(), types.NamespacedName{Name: policy.Name, Namespace: ns}, &policyv1.Policy{})
	require.True(t, apierrors.IsNotFound(err), "Policy must be deleted by full RS cleanup")

	err = c.Get(context.TODO(), types.NamespacedName{Name: placement.Name}, &clusterv1beta1.Placement{})
	require.True(t, apierrors.IsNotFound(err), "Placement must be deleted by full RS cleanup")

	err = c.Get(context.TODO(), types.NamespacedName{Name: cm.Name, Namespace: ns}, &corev1.ConfigMap{})
	require.True(t, apierrors.IsNotFound(err), "ConfigMap must be deleted by full RS cleanup (MCO CR gone)")
}
