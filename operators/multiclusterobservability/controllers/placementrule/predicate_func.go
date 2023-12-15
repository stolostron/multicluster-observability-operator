// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func getClusterPreds() predicate.Funcs {

	createFunc := func(e event.CreateEvent) bool {
		log.Info("CreateFunc", "managedCluster", e.Object.GetName())

		if isAutomaticAddonInstallationDisabled(e.Object) {
			return false
		}

		updateManagedClusterList(e.Object)
		updateManagedClusterImageRegistry(e.Object)

		return true
	}

	updateFunc := func(e event.UpdateEvent) bool {
		log.Info("UpdateFunc", "managedCluster", e.ObjectNew.GetName())

		if e.ObjectNew.GetResourceVersion() == e.ObjectOld.GetResourceVersion() {
			return false
		}
		if isAutomaticAddonInstallationDisabled(e.ObjectNew) {
			return false
		}

		if e.ObjectNew.GetDeletionTimestamp() != nil {
			log.Info("managedcluster is in terminating state", "managedCluster", e.ObjectNew.GetName())
			managedClusterListMutex.Lock()
			delete(managedClusterList, e.ObjectNew.GetName())
			managedClusterListMutex.Unlock()
			managedClusterImageRegistryMutex.Lock()
			delete(managedClusterImageRegistry, e.ObjectNew.GetName())
			managedClusterImageRegistryMutex.Unlock()
		} else {
			updateManagedClusterList(e.ObjectNew)
			updateManagedClusterImageRegistry(e.ObjectNew)
		}

		return true
	}

	deleteFunc := func(e event.DeleteEvent) bool {
		log.Info("DeleteFunc", "managedCluster", e.Object.GetName())

		if isAutomaticAddonInstallationDisabled(e.Object) {
			return false
		}

		managedClusterListMutex.Lock()
		delete(managedClusterList, e.Object.GetName())
		managedClusterListMutex.Unlock()
		managedClusterImageRegistryMutex.Lock()
		delete(managedClusterImageRegistry, e.Object.GetName())
		managedClusterImageRegistryMutex.Unlock()

		return true
	}

	return predicate.Funcs{
		CreateFunc: createFunc,
		UpdateFunc: updateFunc,
		DeleteFunc: deleteFunc,
	}
}

func GetAddOnDeploymentPredicates() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == defaultAddonDeploymentConfig.Name &&
				e.ObjectNew.GetNamespace() == defaultAddonDeploymentConfig.Namespace {
				log.Info("default AddonDeploymentConfig is updated")
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == defaultAddonDeploymentConfig.Name &&
				e.Object.GetNamespace() == defaultAddonDeploymentConfig.Namespace {
				log.Info("default AddonDeploymentConfig is deleted")
				return true
			}
			return false
		},
	}
}
