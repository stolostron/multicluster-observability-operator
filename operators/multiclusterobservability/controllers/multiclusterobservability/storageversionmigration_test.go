// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"testing"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	migrationv1alpha1 "sigs.k8s.io/kube-storage-version-migrator/pkg/apis/migration/v1alpha1"
)

func TestCreateOrUpdateObservabilityStorageVersionMigrationResource(t *testing.T) {
	var (
		name      = "observability"
		namespace = mcoconfig.GetDefaultNamespace()
	)
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       mcov1beta2.MultiClusterObservabilitySpec{},
	}
	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	migrationv1alpha1.SchemeBuilder.AddToScheme(s)

	c := fake.NewClientBuilder().Build()

	// test scenario of creating StorageVersionMigration
	err := createOrUpdateObservabilityStorageVersionMigrationResource(t.Context(), c, s, mco)
	if err != nil {
		t.Fatalf("createOrUpdateObservabilityStorageVersionMigrationResource: (%v)", err)
	}

	// Test scenario in which StorageVersionMigration updated by others
	svmName := storageVersionMigrationPrefix + "-" + mco.GetName()
	svm := &migrationv1alpha1.StorageVersionMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name: svmName,
		},
		Spec: migrationv1alpha1.StorageVersionMigrationSpec{
			Resource: migrationv1alpha1.GroupVersionResource{
				Group:    mcov1beta2.GroupVersion.Group,
				Resource: mcoconfig.MCORsName,
			},
		},
	}
	c = fake.NewClientBuilder().WithRuntimeObjects(svm).Build()
	err = createOrUpdateObservabilityStorageVersionMigrationResource(t.Context(), c, s, mco)
	if err != nil {
		t.Fatalf("createOrUpdateObservabilityStorageVersionMigrationResource: (%v)", err)
	}

	foundSvm := &migrationv1alpha1.StorageVersionMigration{}
	err = c.Get(t.Context(), types.NamespacedName{Name: svmName}, foundSvm)
	if err != nil {
		t.Fatalf("Failed to get StorageVersionMigration (%s): (%v)", svmName, err)
	}
	if foundSvm.Spec.Resource.Version != mcov1beta2.GroupVersion.Version {
		t.Fatalf("Failed to update StorageVersionMigration (%s)", svmName)
	}

	// Test clean scenario in which StorageVersionMigration is already removed
	err = createOrUpdateObservabilityStorageVersionMigrationResource(t.Context(), c, s, mco)
	if err != nil {
		t.Fatalf("Failed to StorageVersionMigration: (%v)", err)
	}

	err = c.Delete(t.Context(), svm)
	if err != nil {
		t.Fatalf("Failed to delete (%s): (%v)", svmName, err)
	}
}
