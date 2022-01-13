// Copyright (c) 2021 Red Hat, Inc.

package placementrule

import (
	"context"
	"testing"
	"time"

	mcov1beta1 "github.com/stolostron/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestObsAddonCR(t *testing.T) {
	initSchema(t)

	objs := []runtime.Object{newTestRoute(), newTestInfra()}
	c := fake.NewFakeClient(objs...)

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
}

func TestStaleObsAddonCR(t *testing.T) {
	initSchema(t)

	objs := []runtime.Object{newTestRoute(), newTestInfra()}
	c := fake.NewFakeClient(objs...)

	err := createObsAddon(c, namespace)
	if err != nil {
		t.Fatalf("Failed to create observabilityaddon: (%v)", err)
	}
	found := &mcov1beta1.ObservabilityAddon{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get observabilityaddon: (%v)", err)
	}

	found.ObjectMeta.DeletionTimestamp = &v1.Time{time.Now()}
	found.SetFinalizers([]string{obsAddonFinalizer})
	err = c.Update(context.TODO(), found)
	if err != nil {
		t.Fatalf("Failed to update observabilityaddon: (%v)", err)
	}

	err = deleteStaleObsAddon(c, namespace)
	if err != nil {
		t.Fatalf("Failed to remove stale observabilityaddon: (%v)", err)
	}
}
