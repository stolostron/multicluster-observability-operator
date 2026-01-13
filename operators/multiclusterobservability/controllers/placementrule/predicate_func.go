// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"

	appsv1 "k8s.io/api/apps/v1"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func getClusterPreds() predicate.Funcs {
	createFunc := func(e event.CreateEvent) bool {
		log.Info("CreateFunc", "managedCluster", e.Object.GetName())

		if isAutomaticAddonInstallationDisabled(e.Object) {
			return false
		}
		updateManagedClusterImageRegistry(e.Object)
		return areManagedClusterLabelsReady(e.Object)
	}

	updateFunc := func(e event.UpdateEvent) bool {
		if e.ObjectNew.GetResourceVersion() == e.ObjectOld.GetResourceVersion() {
			return false
		}
		if isAutomaticAddonInstallationDisabled(e.ObjectNew) {
			return false
		}

		if e.ObjectNew.GetDeletionTimestamp() != nil {
			log.Info("managedcluster is in terminating state", "managedCluster", e.ObjectNew.GetName())
			managedClusterImageRegistryMutex.Lock()
			delete(managedClusterImageRegistry, e.ObjectNew.GetName())
			managedClusterImageRegistryMutex.Unlock()
		} else {
			updateManagedClusterImageRegistry(e.ObjectNew)
			if !areManagedClusterLabelsReady(e.ObjectNew) {
				return false
			}
		}

		return true
	}

	deleteFunc := func(e event.DeleteEvent) bool {
		log.Info("DeleteFunc", "managedCluster", e.Object.GetName())

		if isAutomaticAddonInstallationDisabled(e.Object) {
			return false
		}

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

func GetAddOnDeploymentConfigPredicates() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			newObj := e.ObjectNew.(*addonv1alpha1.AddOnDeploymentConfig)
			oldObj := e.ObjectOld.(*addonv1alpha1.AddOnDeploymentConfig)
			if reflect.DeepEqual(newObj.Spec, oldObj.Spec) {
				return false
			}
			log.Info("AddonDeploymentConfig is updated", e.ObjectNew.GetName(), "name", e.ObjectNew.GetNamespace(), "namespace")
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}
}

func getHubEndpointOperatorPredicates() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() && e.ObjectNew.GetName() == config.HubEndpointOperatorName &&
				!reflect.DeepEqual(e.ObjectNew.(*appsv1.Deployment).Spec.Template.Spec,
					e.ObjectOld.(*appsv1.Deployment).Spec.Template.Spec) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if operatorconfig.IsMCOTerminating {
				log.Info("MCO is terminating, skip reconcile for hub endpoint operator")
				return false
			}
			if e.Object.GetNamespace() == config.GetDefaultNamespace() && e.Object.GetName() == config.HubEndpointOperatorName {
				return true
			}
			return false
		},
	}
}

//nolint:unparam
func getPred(name string, namespace string,
	create bool, update bool, isDelete bool) predicate.Funcs {
	createFunc := func(e event.CreateEvent) bool {
		return false
	}
	updateFunc := func(e event.UpdateEvent) bool {
		return false
	}
	deleteFunc := func(e event.DeleteEvent) bool {
		return false
	}
	if create {
		createFunc = func(e event.CreateEvent) bool {
			if e.Object.GetName() == name && (e.Object.GetNamespace() == namespace) {
				return true
			}
			return false
		}
	}
	if update {
		updateFunc = func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == name && (e.ObjectNew.GetNamespace() == namespace) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				// also check objectNew string in case Kind is empty
				if strings.HasPrefix(fmt.Sprint(e.ObjectNew), "&Deployment") ||
					e.ObjectNew.GetObjectKind().GroupVersionKind().Kind == "Deployment" {
					if !reflect.DeepEqual(e.ObjectNew.(*appsv1.Deployment).Spec.Template.Spec,
						e.ObjectOld.(*appsv1.Deployment).Spec.Template.Spec) {
						return true
					}
				} else {
					return true
				}
			}
			return false
		}
	}
	if isDelete {
		deleteFunc = func(e event.DeleteEvent) bool {
			if operatorconfig.IsMCOTerminating {
				log.Info("MCO is terminating, skip reconcile for placementrule controller", "name", name, "namespace", namespace)
				return false
			}
			if e.Object.GetName() == name && (e.Object.GetNamespace() == namespace) {
				return true
			}
			return false
		}
	}
	return predicate.Funcs{
		CreateFunc: createFunc,
		UpdateFunc: updateFunc,
		DeleteFunc: deleteFunc,
	}
}
