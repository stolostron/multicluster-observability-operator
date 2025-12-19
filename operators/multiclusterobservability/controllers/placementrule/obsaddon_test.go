// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	obshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
)

func TestObsAddonCR(t *testing.T) {
	initSchema(t)

	objs := []runtime.Object{newTestObsApiRoute()}
	c := fake.NewClientBuilder().
		WithRuntimeObjects(objs...).
		WithStatusSubresource(
			&addonv1alpha1.ManagedClusterAddOn{},
			&mcov1beta2.MultiClusterObservability{},
			&mcov1beta1.ObservabilityAddon{},
		).
		Build()

	err := createObsAddon(&mcov1beta2.MultiClusterObservability{}, c, namespace)
	if err != nil {
		t.Fatalf("Failed to create observabilityaddon: (%v)", err)
	}
	found := &mcov1beta1.ObservabilityAddon{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get observabilityaddon: (%v)", err)
	}

	err = createObsAddon(&mcov1beta2.MultiClusterObservability{}, c, namespace)
	if err != nil {
		t.Fatalf("Failed to create observabilityaddon: (%v)", err)
	}

	testWork := newManifestwork(namespace+workNameSuffix, namespace)
	testManifests := testWork.Spec.Workload.Manifests
	testObservabilityAddon := &mcov1beta1.ObservabilityAddon{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, testObservabilityAddon)
	if err != nil {
		t.Fatalf("Failed to get observabilityaddon: (%v)", err)
	}
	// inject the testing observabilityAddon
	testManifests = injectIntoWork(testManifests, testObservabilityAddon)
	testWork.Spec.Workload.Manifests = testManifests

	err = c.Create(context.TODO(), testWork)
	if err != nil {
		t.Fatalf("Failed to create manifestwork: (%v)", err)
	}

	_, err = deleteObsAddon(context.Background(), c, namespace)
	if err != nil {
		t.Fatalf("Failed to delete observabilityaddon: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Failed to delete observabilityaddon: (%v)", err)
	}

	_, err = deleteObsAddon(context.Background(), c, namespace)
	if err != nil {
		t.Fatalf("Failed to delete observabilityaddon: (%v)", err)
	}

	err = deleteManifestWork(c, namespace+workNameSuffix, namespace)
	if err != nil {
		t.Fatalf("Failed to delete manifestwork: (%v)", err)
	}
}

func TestStaleObsAddonCR(t *testing.T) {
	initSchema(t)

	deletetionTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	addon := &mcov1beta1.ObservabilityAddon{
		ObjectMeta: metav1.ObjectMeta{
			Name:              obsAddonName,
			Namespace:         namespace,
			DeletionTimestamp: &deletetionTime,
			Finalizers:        []string{obsAddonFinalizer},
		},
	}
	c := fake.NewClientBuilder().WithRuntimeObjects(addon).Build()

	if _, err := deleteObsAddonObject(context.Background(), c, namespace); err != nil {
		t.Fatalf("Failed to remove stale observabilityaddon: (%v)", err)
	}

	found := &mcov1beta1.ObservabilityAddon{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err == nil {
		t.Fatalf("Failed to delete observabilityaddon, still present")
	}
	if err != nil && !errors.IsNotFound(err) {
		t.Fatalf("Failed to delete observabilityaddon: %v", err)
	}

}

func TestSetObservabilityAddonSpec(t *testing.T) {
	tests := []struct {
		name        string
		desiredSpec *obshared.ObservabilityAddonSpec
		resources   *corev1.ResourceRequirements
		want        *mcov1beta1.ObservabilityAddon
	}{
		{
			name: "with desired spec",
			desiredSpec: &obshared.ObservabilityAddonSpec{
				EnableMetrics:        true,
				Interval:             60,
				ScrapeSizeLimitBytes: 1024,
				Workers:              2,
			},
			resources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
			want: &mcov1beta1.ObservabilityAddon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      obsAddonName,
					Namespace: namespace,
					Annotations: map[string]string{
						"test": "test",
					},
				},
				Spec: obshared.ObservabilityAddonSpec{
					EnableMetrics:        true,
					Interval:             60,
					ScrapeSizeLimitBytes: 1024,
					Workers:              2,
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
		{
			name:        "with nil desired spec",
			desiredSpec: nil,
			resources:   nil,
			want: &mcov1beta1.ObservabilityAddon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      obsAddonName,
					Namespace: namespace,
					Annotations: map[string]string{
						"test": "test",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := &mcov1beta1.ObservabilityAddon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      obsAddonName,
					Namespace: namespace,
					Annotations: map[string]string{
						"test": "test",
					},
				},
			}

			setObservabilityAddonSpec(got, tt.desiredSpec, tt.resources)

			if !equality.Semantic.DeepEqual(got, tt.want) {
				t.Errorf("setObservabilityAddonSpec() = %v, want %v", got, tt.want)
			}

			override := &mcov1beta1.ObservabilityAddon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      obsAddonName,
					Namespace: namespace,
					Annotations: map[string]string{
						"test": "test",
					},
				},
				Spec: obshared.ObservabilityAddonSpec{
					EnableMetrics:        true,
					Interval:             50,
					ScrapeSizeLimitBytes: 24,
					Workers:              1,
				},
			}
			setObservabilityAddonSpec(override, tt.desiredSpec, tt.resources)

			if tt.name != "with nil desired spec" {
				if !equality.Semantic.DeepEqual(override, tt.want) {
					t.Errorf("setObservabilityAddonSpec() = %v, want %v", override, tt.want)
				}
			}
		})
	}
}
