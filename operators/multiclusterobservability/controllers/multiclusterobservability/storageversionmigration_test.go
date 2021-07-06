// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	migrationv1alpha1 "sigs.k8s.io/kube-storage-version-migrator/pkg/apis/migration/v1alpha1"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
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

	c := fake.NewFakeClient()

	// test scenario of creating StorageVersionMigration
	err := createOrUpdateObservabilityStorageVersionMigrationResource(c, s, mco)
	if err != nil {
		t.Fatalf("createOrUpdateObservabilityStorageVersionMigrationResource: (%v)", err)
	}

	// Test scenario in which StorageVersionMigration updated by others
	svmName := storageVersionMigrationPrefix + mco.GetName()
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
	c = fake.NewFakeClient(svm)
	err = createOrUpdateObservabilityStorageVersionMigrationResource(c, s, mco)
	if err != nil {
		t.Fatalf("createOrUpdateObservabilityStorageVersionMigrationResource: (%v)", err)
	}

	foundSvm := &migrationv1alpha1.StorageVersionMigration{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: svmName}, foundSvm)
	if err != nil {
		t.Fatalf("Failed to get StorageVersionMigration (%s): (%v)", svmName, err)
	}
	if foundSvm.Spec.Resource.Version != mcov1beta2.GroupVersion.Version {
		t.Fatalf("Failed to update StorageVersionMigration (%s)", svmName)
	}

	err = cleanObservabilityStorageVersionMigrationResource(c, mco)
	if err != nil {
		t.Fatalf("Failed to clean the StorageVersionMigration")
	}

	// Test clean scenario in which StorageVersionMigration is already removed
	err = createOrUpdateObservabilityStorageVersionMigrationResource(c, s, mco)
	if err != nil {
		t.Fatalf("Failed to StorageVersionMigration: (%v)", err)
	}

	err = c.Delete(context.TODO(), svm)
	if err != nil {
		t.Fatalf("Failed to delete (%s): (%v)", svmName, err)
	}

	err = cleanObservabilityStorageVersionMigrationResource(c, mco)
	if err != nil {
		t.Fatalf("Failed to clean the StorageVersionMigration")
	}
}
