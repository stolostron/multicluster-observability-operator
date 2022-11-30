// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"reflect"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
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
			if !reflect.DeepEqual(oldDefault, newDefault) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}

func getMgClusterAddonPredFunc() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
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
			if !reflect.DeepEqual(oldConfig, newConfig) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}
