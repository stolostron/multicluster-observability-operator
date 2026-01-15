// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"context"
	"reflect"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	migrationv1alpha1 "sigs.k8s.io/kube-storage-version-migrator/pkg/apis/migration/v1alpha1"
)

var storageVersionMigrationPrefix = "storage-version-migration"

// createOrUpdateObservabilityStorageVersionMigrationResource create or update the StorageVersionMigration resource
func createOrUpdateObservabilityStorageVersionMigrationResource(client client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
) error {
	storageVersionMigrationName := storageVersionMigrationPrefix
	if mco != nil {
		storageVersionMigrationName += "-" + mco.GetName()
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

	if err := controllerutil.SetControllerReference(mco, storageVersionMigration, scheme); err != nil {
		log.Error(err, "Failed to set controller reference", "name", storageVersionMigrationName)
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
		storageVersionMigration.ResourceVersion = found.ResourceVersion
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
