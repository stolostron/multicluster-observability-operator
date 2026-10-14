// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"
	"testing"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestIndependentRightSizingDefaultsAndADC(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		ns                    *bool // namespace RS — nil means omitted (defaults to true)
		workload, pod         *bool
		wantWorkload, wantPod bool
	}{
		// Fresh 5.1 install: everything omitted → all default to true.
		{"fresh install: all omitted", nil, nil, nil, true, true},

		// Upgrade from 2.17/5.0 where the operator already persisted ns/virt.
		// Workload/pod are new in 5.1 and inherit from the namespace switch.
		{"upgrade: ns enabled → workload/pod inherit on", ptr.To(true), nil, nil, true, true},
		{"upgrade: ns disabled → workload/pod inherit off", ptr.To(false), nil, nil, false, false},

		// Explicit per-feature overrides.
		{"ns disabled but workload explicit on", ptr.To(false), ptr.To(true), nil, true, false},
		{"workload disabled", nil, ptr.To(false), nil, false, true},
		{"pod disabled", nil, nil, ptr.To(false), true, false},
		{"both disabled", nil, ptr.To(false), ptr.To(false), false, false},
		{"both enabled", nil, ptr.To(true), ptr.To(true), true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mco := &unstructured.Unstructured{}
			mco.SetGroupVersionKind(mcoGVK)
			mco.SetName("observability")
			for key, value := range map[string]*bool{
				"namespaceRightSizingRecommendation": tt.ns,
				"workloadRightSizingRecommendation":  tt.workload,
				"podRightSizingRecommendation":       tt.pod,
			} {
				if value != nil {
					require.NoError(t, unstructured.SetNestedField(mco.Object, *value, "spec", "capabilities", "platform", "analytics", key, "enabled"))
				}
			}
			adc := &addonv1beta1.AddOnDeploymentConfig{ObjectMeta: metav1.ObjectMeta{Name: config.MultiClusterObservabilityAddon, Namespace: config.GetDefaultNamespace()}}
			c := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).WithObjects(mco, adc).
				WithInterceptorFuncs(interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if err := c.Get(ctx, key, obj, opts...); err != nil {
						return err
					}
					// The fake tracker round-trips through the typed bool/omitempty
					// API and drops explicit false; preserve the API-server payload.
					raw, ok := obj.(*unstructured.Unstructured)
					if !ok {
						return nil
					}
					if tt.ns != nil {
						if err := unstructured.SetNestedField(raw.Object, *tt.ns, "spec", "capabilities", "platform", "analytics", "namespaceRightSizingRecommendation", "enabled"); err != nil {
							return err
						}
					}
					return nil
				}}).Build()
			r := &AnalyticsReconciler{Client: c, Log: ctrl.Log.WithName("test")}
			instance := &mcov1beta2.MultiClusterObservability{}
			require.NoError(t, c.Get(t.Context(), types.NamespacedName{Name: mco.GetName()}, instance))
			for range 2 {
				updated, err := r.ensureRightSizingDefaults(t.Context(), instance, r.Log)
				require.NoError(t, err)
				require.Equal(t, tt.wantWorkload, updated.Spec.Capabilities.Platform.Analytics.WorkloadRightSizingRecommendation.IsEnabled())
				require.Equal(t, tt.wantPod, updated.Spec.Capabilities.Platform.Analytics.PodRightSizingRecommendation.IsEnabled())
				require.NoError(t, c.Get(t.Context(), types.NamespacedName{Name: mco.GetName()}, instance))
				require.Equal(t, ptr.To(tt.wantWorkload), instance.Spec.Capabilities.Platform.Analytics.WorkloadRightSizingRecommendation.Enabled)
				require.Equal(t, ptr.To(tt.wantPod), instance.Spec.Capabilities.Platform.Analytics.PodRightSizingRecommendation.Enabled)
			}
			for _, delegated := range []bool{true, false} {
				require.NoError(t, r.syncRightSizingStateToADC(t.Context(), instance, delegated, r.Log))
				require.NoError(t, c.Get(t.Context(), types.NamespacedName{Name: adc.Name, Namespace: adc.Namespace}, adc))
				values := map[string]string{}
				for _, v := range adc.Spec.CustomizedVariables {
					values[v.Name] = v.Value
				}
				for key, enabled := range map[string]bool{util.ADCKeyPlatformWorkloadRightSizing: tt.wantWorkload, util.ADCKeyPlatformPodRightSizing: tt.wantPod} {
					want := "disabled"
					if delegated && enabled {
						want = "enabled"
					}
					require.Equal(t, want, values[key])
				}
			}
		})
	}
}
