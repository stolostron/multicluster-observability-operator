// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"testing"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHasMCOAManifestWorks(t *testing.T) {
	s := scheme.Scheme
	_ = addonv1beta1.Install(s)
	_ = workv1.Install(s)

	t.Run("returns blocking namespaces when healthy available cluster has ManifestWorks", func(t *testing.T) {
		mw := &workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "addon-multicluster-observability-addon-deploy-0",
				Namespace: "healthy-cluster",
				Labels: map[string]string{
					addonv1beta1.AddonLabelKey: config.MultiClusterObservabilityAddon,
				},
			},
		}
		// In production, ManagedClusterAddOn resources do not necessarily carry labels.
		// We explicitly leave it unlabeled to simulate this.
		addon := &addonv1beta1.ManagedClusterAddOn{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.MultiClusterObservabilityAddon,
				Namespace: "healthy-cluster",
			},
			Status: addonv1beta1.ManagedClusterAddOnStatus{
				Conditions: []metav1.Condition{
					{
						Type:   "Available",
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(mw, addon).Build()
		blocking, err := HasMCOAManifestWorks(context.Background(), cl)
		assert.NoError(t, err)
		assert.Contains(t, blocking, "healthy-cluster", "Expected ManifestWorks to be detected for healthy available cluster")
	})

	t.Run("returns empty slice when ManifestWork is in a disconnected/offline cluster", func(t *testing.T) {
		mw := &workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "addon-multicluster-observability-addon-deploy-0",
				Namespace: "offline-cluster",
				Labels: map[string]string{
					addonv1beta1.AddonLabelKey: config.MultiClusterObservabilityAddon,
				},
			},
		}
		// In production, ManagedClusterAddOn resources do not necessarily carry labels.
		// We explicitly leave it unlabeled to simulate this.
		addon := &addonv1beta1.ManagedClusterAddOn{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.MultiClusterObservabilityAddon,
				Namespace: "offline-cluster",
			},
			Status: addonv1beta1.ManagedClusterAddOnStatus{
				Conditions: []metav1.Condition{
					{
						Type:   "Available",
						Status: metav1.ConditionFalse, // Degraded/Offline
					},
				},
			},
		}

		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(mw, addon).Build()
		blocking, err := HasMCOAManifestWorks(context.Background(), cl)
		assert.NoError(t, err)
		assert.Empty(t, blocking, "Expected ManifestWorks from offline cluster to be ignored")
	})

	t.Run("returns blocking namespaces when ManifestWork exists but no ManagedClusterAddOn exists (backward-compatibility / simple tests)", func(t *testing.T) {
		mw := &workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "addon-multicluster-observability-addon-deploy-0",
				Namespace: "test-cluster",
				Labels: map[string]string{
					addonv1beta1.AddonLabelKey: config.MultiClusterObservabilityAddon,
				},
			},
		}

		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(mw).Build()
		blocking, err := HasMCOAManifestWorks(context.Background(), cl)
		assert.NoError(t, err)
		assert.Contains(t, blocking, "test-cluster", "Expected ManifestWorks to be detected when no ManagedClusterAddOn is mocked")
	})
}
