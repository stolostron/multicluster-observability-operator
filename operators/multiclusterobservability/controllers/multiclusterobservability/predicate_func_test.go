// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"testing"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestGetMCHPredicateFunc(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "open-cluster-management-observability")

	s := scheme.Scheme
	c := fake.NewClientBuilder().WithScheme(s).Build()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mco-image-manifest-1.0",
			Namespace: config.GetMCONamespace(),
			Labels: map[string]string{
				config.OCMManifestConfigMapTypeLabelKey:    config.OCMManifestConfigMapTypeLabelValue,
				config.OCMManifestConfigMapVersionLabelKey: "1.0",
			},
		},
		Data: map[string]string{
			"key": "value",
		},
	}
	if err := c.Create(t.Context(), cm); err != nil {
		t.Fatalf("Failed to create configmap: %v", err)
	}

	mchPred := GetMCHPredicateFunc(c)

	mchObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": config.MCHGroup + "/" + config.MCHVersion,
			"kind":       config.MCHKind,
			"metadata": map[string]interface{}{
				"name":            "mch",
				"namespace":       config.GetMCONamespace(),
				"resourceVersion": "1",
			},
			"status": map[string]interface{}{
				"currentVersion": "1.0",
				"desiredVersion": "1.0",
			},
		},
	}

	createEvent := event.CreateEvent{Object: mchObj}
	if !mchPred.Create(createEvent) {
		t.Error("mch Create function should return true")
	}

	oldMch := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": config.MCHGroup + "/" + config.MCHVersion,
			"kind":       config.MCHKind,
			"metadata": map[string]interface{}{
				"name":            "mch",
				"namespace":       config.GetMCONamespace(),
				"resourceVersion": "0",
			},
		},
	}
	updateEvent := event.UpdateEvent{ObjectOld: oldMch, ObjectNew: mchObj}
	if !mchPred.Update(updateEvent) {
		t.Error("mch Update function should return true")
	}

	deleteEvent := event.DeleteEvent{Object: mchObj}
	if mchPred.Delete(deleteEvent) {
		t.Error("mch Delete function should return false")
	}

	genericEvent := event.GenericEvent{Object: mchObj}
	if !mchPred.Generic(genericEvent) {
		t.Error("mch Generic function should return true")
	}

	// Test !ok branch (wrong type)
	invalidObj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: config.GetMCONamespace(),
		},
	}

	createEventInvalid := event.CreateEvent{Object: invalidObj}
	if mchPred.Create(createEventInvalid) {
		t.Error("Create event with invalid object type should return false")
	}

	updateEventInvalid := event.UpdateEvent{ObjectOld: invalidObj, ObjectNew: invalidObj}
	if mchPred.Update(updateEventInvalid) {
		t.Error("Update event with invalid object type should return false")
	}
}

func TestGetMCOPredicateFunc(t *testing.T) {
	mcoPred := GetMCOPredicateFunc()

	mcoObj := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mco",
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{},
		},
	}

	createEvent := event.CreateEvent{Object: mcoObj}
	if !mcoPred.Create(createEvent) {
		t.Error("mco Create function should return true")
	}

	updateEvent := event.UpdateEvent{ObjectOld: mcoObj, ObjectNew: mcoObj}
	if mcoPred.Update(updateEvent) {
		t.Error("mco Update function should return false when resource version is unchanged")
	}

	deleteEvent := event.DeleteEvent{Object: mcoObj}
	if !mcoPred.Delete(deleteEvent) {
		t.Error("mco Delete function should return true")
	}
}
