// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"reflect"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func GetMCOPredicateFunc() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			//set request name to be used in placementrule controller
			config.SetMonitoringCRName(e.Object.GetName())
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			checkStorageChanged(e.ObjectOld.(*mcov1beta2.MultiClusterObservability).Spec.StorageConfig,
				e.ObjectNew.(*mcov1beta2.MultiClusterObservability).Spec.StorageConfig)
			return e.ObjectOld.GetResourceVersion() != e.ObjectNew.GetResourceVersion()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !e.DeleteStateUnknown
		},
	}
}

func GetConfigMapPredicateFunc() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() {
				if e.Object.GetName() == config.AlertRuleCustomConfigMapName {
					return true
				} else if _, ok := e.Object.GetLabels()[config.BackupLabelName]; ok {
					// resource already has backup label
					return false
				} else if _, ok := config.BackupResourceMap[e.Object.GetName()]; ok {
					// resource's backup label must be checked
					return true
				} else if _, ok := e.Object.GetLabels()[config.GrafanaCustomDashboardLabel]; ok {
					// ConfigMap with custom-grafana-dashboard labels, check for backup label
					config.BackupResourceMap[e.Object.GetName()] = config.ResourceTypeConfigMap
					return true
				}
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Find a way to restart the alertmanager to take the update
			if e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() {
				if e.ObjectNew.GetName() == config.AlertRuleCustomConfigMapName {
					// Grafana dynamically loads AlertRule configmap, nothing more to do
					//config.SetCustomRuleConfigMap(true)
					//return e.ObjectOld.GetResourceVersion() != e.ObjectNew.GetResourceVersion()
					return false
				} else if _, ok := e.ObjectNew.GetLabels()[config.BackupLabelName]; ok {
					// resource already has backup label
					return false
				} else if _, ok := config.BackupResourceMap[e.ObjectNew.GetName()]; ok {
					// resource's backup label must be checked
					return true
				} else if _, ok := e.ObjectNew.GetLabels()[config.GrafanaCustomDashboardLabel]; ok {
					// ConfigMap with custom-grafana-dashboard labels, check for backup label
					config.BackupResourceMap[e.ObjectNew.GetName()] = config.ResourceTypeConfigMap
					return true
				}
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == config.AlertRuleCustomConfigMapName &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return true
			}
			return false
		},
	}
}

func GetAlertManagerSecretPredicateFunc() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() {
				if e.Object.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.Object.GetName() == config.AlertmanagerRouteBYOCERTName {
					return true
				} else if _, ok := e.Object.GetLabels()[config.BackupLabelName]; ok {
					// resource already has backup label
					return false
				} else if _, ok := config.BackupResourceMap[e.Object.GetName()]; ok {
					// resource's backup label must be checked
					return true
				}
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() {
				if e.ObjectNew.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.ObjectNew.GetName() == config.AlertmanagerRouteBYOCERTName {
					return true
				} else if _, ok := e.ObjectNew.GetLabels()[config.BackupLabelName]; ok {
					// resource already has backup label
					return false
				} else if _, ok := config.BackupResourceMap[e.ObjectNew.GetName()]; ok {
					// resource's backup label must be checked
					return true
				}
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() &&
				(e.Object.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.Object.GetName() == config.AlertmanagerRouteBYOCERTName ||
					e.Object.GetName() == config.AlertmanagerConfigName) {
				return true
			}
			return false
		},
	}
}

func GetMCHPredicateFunc(c client.Client) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// this is for operator restart, the mch CREATE event will be caught and the mch should be ready
			if e.Object.GetNamespace() == config.GetMCONamespace() &&
				e.Object.(*mchv1.MultiClusterHub).Status.CurrentVersion != "" &&
				e.Object.(*mchv1.MultiClusterHub).Status.DesiredVersion == e.Object.(*mchv1.MultiClusterHub).Status.CurrentVersion {
				// only read the image manifests configmap and enqueue the request when the MCH is
				// installed/upgraded successfully
				_, ok, err := config.ReadImageManifestConfigMap(
					c,
					e.Object.(*mchv1.MultiClusterHub).Status.CurrentVersion,
				)
				if err != nil {
					return false
				}
				return ok
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ensure the event pertains to the target namespace and object type
			if e.ObjectNew.GetNamespace() == config.GetMCONamespace() &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
				e.ObjectNew.(*mchv1.MultiClusterHub).Status.CurrentVersion != "" &&
				e.ObjectNew.(*mchv1.MultiClusterHub).Status.DesiredVersion ==
					e.ObjectNew.(*mchv1.MultiClusterHub).Status.CurrentVersion {

				currentData, _, err := config.ReadImageManifestConfigMap(
					c,
					e.ObjectNew.(*mchv1.MultiClusterHub).Status.CurrentVersion,
				)
				if err != nil {
					log.Error(err, "Failed to read image manifest ConfigMap")
					return false
				}

				previousData, exists := config.GetCachedImageManifestData()
				if !exists {
					config.SetCachedImageManifestData(currentData)
					return true
				}
				if !reflect.DeepEqual(currentData, previousData) {
					config.SetCachedImageManifestData(currentData)
					return true
				}
				return false
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}

func GetNamespacePredicateFunc() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Object.GetName() == config.GetDefaultNamespace()
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			labelVal, labelExists := e.ObjectNew.GetLabels()[config.OpenShiftClusterMonitoringlabel]
			shouldReconcile := !labelExists || (labelExists && labelVal != "true")
			return e.ObjectNew.GetName() == config.GetDefaultNamespace() && shouldReconcile
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}
