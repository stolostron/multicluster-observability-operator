// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package placementrule

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func getClusterPred(c client.Client, ingressCtlCrdExists bool) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			log.Info("CreateFunc", "managedCluster", e.Object.GetName())
			updateManagedClusterList(e.Object)
			updateManagedClusterImageRegistry(e.Object)
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			log.Info("UpdateFunc", "managedCluster", e.ObjectNew.GetName())
			if e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
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
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			log.Info("DeleteFunc", "managedCluster", e.Object.GetName())
			managedClusterListMutex.Lock()
			delete(managedClusterList, e.Object.GetName())
			managedClusterListMutex.Unlock()
			managedClusterImageRegistryMutex.Lock()
			delete(managedClusterImageRegistry, e.Object.GetName())
			managedClusterImageRegistryMutex.Unlock()
			return true
		},
	}
}
