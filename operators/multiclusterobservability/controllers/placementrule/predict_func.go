// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"reflect"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func getClusterMgmtAddonPredFunc() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld.GetName() != util.ObservabilityController {
				return false
			}
			oldDefault := &addonv1alpha1.ConfigReferent{}
			newDefault := &addonv1alpha1.ConfigReferent{}
			for _, config := range e.ObjectOld.(*addonv1alpha1.ClusterManagementAddOn).Spec.SupportedConfigs {
				if config.ConfigGroupResource.Group == util.AddonGroup &&
					config.ConfigGroupResource.Resource == util.AddonDeploymentConfigResource {
					oldDefault = config.DefaultConfig
				}
			}
			for _, config := range e.ObjectNew.(*addonv1alpha1.ClusterManagementAddOn).Spec.SupportedConfigs {
				if config.ConfigGroupResource.Group == util.AddonGroup &&
					config.ConfigGroupResource.Resource == util.AddonDeploymentConfigResource {
					newDefault = config.DefaultConfig
				}
			}
			return !reflect.DeepEqual(oldDefault, newDefault)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}

func getMgClusterAddonPredFunc() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Object.GetName() == util.ManagedClusterAddonName
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld.GetName() != util.ManagedClusterAddonName {
				return false
			}
			oldConfig := addonv1alpha1.ConfigReferent{}
			newConfig := addonv1alpha1.ConfigReferent{}
			for _, config := range e.ObjectOld.(*addonv1alpha1.ManagedClusterAddOn).Spec.Configs {
				if config.ConfigGroupResource.Group == util.AddonGroup &&
					config.ConfigGroupResource.Resource == util.AddonDeploymentConfigResource {
					oldConfig = config.ConfigReferent
				}
			}
			for _, config := range e.ObjectNew.(*addonv1alpha1.ManagedClusterAddOn).Spec.Configs {
				if config.ConfigGroupResource.Group == util.AddonGroup &&
					config.ConfigGroupResource.Resource == util.AddonDeploymentConfigResource {
					newConfig = config.ConfigReferent
				}
			}
			return !reflect.DeepEqual(oldConfig, newConfig)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}

func getManifestworkPred() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetLabels()[ownerLabelKey] == ownerLabelValue &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
				!reflect.DeepEqual(e.ObjectNew.(*workv1.ManifestWork).Spec.Workload.Manifests,
					e.ObjectOld.(*workv1.ManifestWork).Spec.Workload.Manifests) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.GetLabels()[ownerLabelKey] == ownerLabelValue
		},
	}

}

func getMchPred(c client.Client) predicate.Funcs {
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
