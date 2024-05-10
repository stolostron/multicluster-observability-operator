// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"testing"
	"time"

	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

	err := createObsAddon(c, namespace)
	if err != nil {
		t.Fatalf("Failed to create observabilityaddon: (%v)", err)
	}
	found := &mcov1beta1.ObservabilityAddon{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get observabilityaddon: (%v)", err)
	}

	err = createObsAddon(c, namespace)
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

	err = deleteObsAddon(c, namespace)
	if err != nil {
		t.Fatalf("Failed to delete observabilityaddon: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Failed to delete observabilityaddon: (%v)", err)
	}

	err = deleteObsAddon(c, namespace)
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

	objs := []runtime.Object{newTestObsApiRoute()}
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()

	err := createObsAddon(c, namespace)
	if err != nil {
		t.Fatalf("Failed to create observabilityaddon: (%v)", err)
	}
	found := &mcov1beta1.ObservabilityAddon{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get observabilityaddon: (%v)", err)
	}

	found.ObjectMeta.DeletionTimestamp = &v1.Time{Time: time.Now()}
	found.SetFinalizers([]string{obsAddonFinalizer})
	err = c.Update(context.TODO(), found)
	if err != nil {
		t.Fatalf("Failed to update observabilityaddon: (%v)", err)
	}

	err = deleteStaleObsAddon(c, namespace, true)
	if err != nil {
		t.Fatalf("Failed to remove stale observabilityaddon: (%v)", err)
	}
}
