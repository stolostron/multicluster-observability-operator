// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	migrationv1alpha1 "sigs.k8s.io/kube-storage-version-migrator/pkg/apis/migration/v1alpha1"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

var (
	storageVersionMigrationPrefix = "storage-version-migration"
)

// createOrUpdateObservabilityStorageVersionMigrationResource create or update the StorageVersionMigration resource
func createOrUpdateObservabilityStorageVersionMigrationResource(client client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) error {
	storageVersionMigrationName := storageVersionMigrationPrefix
	if mco != nil {
		storageVersionMigrationName += mco.GetName()
	}
	storageVersionMigration := &migrationv1alpha1.StorageVersionMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name: storageVersionMigrationName,
		},
		Spec: migrationv1alpha1.StorageVersionMigrationSpec{
			Resource: migrationv1alpha1.GroupVersionResource{
				Group:    mcov1beta2.GroupVersion.Group,
				Version:  mcov1beta2.GroupVersion.Version,
				Resource: config.MCORsName,
			},
		},
	}

	found := &migrationv1alpha1.StorageVersionMigration{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: storageVersionMigrationName}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating StorageVersionMigration", "name", storageVersionMigrationName)
		err = client.Create(context.TODO(), storageVersionMigration)
		if err != nil {
			log.Error(err, "Failed to create StorageVersionMigration", "name", storageVersionMigrationName)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check StorageVersionMigration", "name", storageVersionMigrationName)
		return err
	}

	if !reflect.DeepEqual(found.Spec, storageVersionMigration.Spec) {
		log.Info("Updating StorageVersionMigration", "name", storageVersionMigrationName)
		storageVersionMigration.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = client.Update(context.TODO(), storageVersionMigration)
		if err != nil {
			log.Error(err, "Failed to update StorageVersionMigration", "name", storageVersionMigrationName)
			return err
		}
		return nil
	}

	log.Info("StorageVersionMigration already existed/unchanged", "name", storageVersionMigrationName)
	return nil
}

// cleanObservabilityStorageVersionMigrationResource delete the StorageVersionMigration source if found
func cleanObservabilityStorageVersionMigrationResource(client client.Client, mco *mcov1beta2.MultiClusterObservability) error {
	storageVersionMigrationName := storageVersionMigrationPrefix
	if mco != nil {
		storageVersionMigrationName += mco.GetName()
	}
	found := &migrationv1alpha1.StorageVersionMigration{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: storageVersionMigrationName}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("StorageVersionMigration doesn't exist", "name", storageVersionMigrationName)
	} else if err != nil {
		log.Error(err, "Failed to check StorageVersionMigration", "name", storageVersionMigrationName)
		return err
	} else {
		err = client.Delete(context.TODO(), found)
		if err != nil {
			log.Error(err, "Failed to delete StorageVersionMigration", "name", storageVersionMigrationName)
			return err
		}
	}
	return nil
}
