// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"fmt"
	"maps"
	"reflect"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
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

			oldCMA := e.ObjectOld.(*addonv1beta1.ClusterManagementAddOn)
			newCMA := e.ObjectNew.(*addonv1beta1.ClusterManagementAddOn)

			// Check if spec.supportedConfigs[].defaultConfig changed
			oldDefault := addonv1beta1.ConfigReferent{}
			newDefault := addonv1beta1.ConfigReferent{}
			for _, config := range oldCMA.Spec.DefaultConfigs {
				if config.Group == util.AddonGroup &&
					config.Resource == util.AddonDeploymentConfigResource {
					oldDefault = config.ConfigReferent
				}
			}
			for _, config := range newCMA.Spec.DefaultConfigs {
				if config.Group == util.AddonGroup &&
					config.Resource == util.AddonDeploymentConfigResource {
					newDefault = config.ConfigReferent
				}
			}
			if oldDefault != newDefault {
				return true
			}

			// Also check if status.defaultConfigReferences changed (addon-framework updates specHash here)
			// This ensures ManifestWork annotation gets updated when framework populates specHash
			oldStatusConfig := findAddonDeploymentDefaultConfigReference(oldCMA.Status.DefaultConfigReferences)
			newStatusConfig := findAddonDeploymentDefaultConfigReference(newCMA.Status.DefaultConfigReferences)
			return !reflect.DeepEqual(oldStatusConfig, newStatusConfig)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}

// findAddonDeploymentConfigReference finds the AddOnDeploymentConfig reference in MCA configReferences.
func findAddonDeploymentConfigReference(configRefs []addonv1beta1.ConfigReference) *addonv1beta1.ConfigReference {
	for i := range configRefs {
		if configRefs[i].Group == util.AddonGroup &&
			configRefs[i].Resource == util.AddonDeploymentConfigResource {
			return &configRefs[i]
		}
	}
	return nil
}

// findAddonDeploymentDefaultConfigReference finds the AddOnDeploymentConfig reference in CMA defaultConfigReferences.
func findAddonDeploymentDefaultConfigReference(configRefs []addonv1beta1.DefaultConfigReference) *addonv1beta1.DefaultConfigReference {
	for i := range configRefs {
		if configRefs[i].Group == util.AddonGroup &&
			configRefs[i].Resource == util.AddonDeploymentConfigResource {
			return &configRefs[i]
		}
	}
	return nil
}

func getMgClusterAddonPredFunc() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Object.GetName() == config.ManagedClusterAddonName
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld.GetName() != config.ManagedClusterAddonName {
				return false
			}

			oldMCA := e.ObjectOld.(*addonv1beta1.ManagedClusterAddOn)
			newMCA := e.ObjectNew.(*addonv1beta1.ManagedClusterAddOn)

			// Check if spec.configs changed
			oldConfig := addonv1beta1.ConfigReferent{}
			newConfig := addonv1beta1.ConfigReferent{}
			for _, config := range oldMCA.Spec.Configs {
				if config.Group == util.AddonGroup &&
					config.Resource == util.AddonDeploymentConfigResource {
					oldConfig = config.ConfigReferent
				}
			}
			for _, config := range newMCA.Spec.Configs {
				if config.Group == util.AddonGroup &&
					config.Resource == util.AddonDeploymentConfigResource {
					newConfig = config.ConfigReferent
				}
			}
			if !reflect.DeepEqual(oldConfig, newConfig) {
				return true
			}

			// Also check if status.configReferences changed (addon-framework updates specHash here)
			// This ensures ManifestWork annotation gets updated when framework populates specHash
			oldStatusConfig := findAddonDeploymentConfigReference(oldMCA.Status.ConfigReferences)
			newStatusConfig := findAddonDeploymentConfigReference(newMCA.Status.ConfigReferences)
			return !reflect.DeepEqual(oldStatusConfig, newStatusConfig)
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
			if e.ObjectNew.GetLabels()[ownerLabelKey] != ownerLabelValue {
				return false
			}

			if e.ObjectNew.GetResourceVersion() == e.ObjectOld.GetResourceVersion() {
				return false
			}

			if !reflect.DeepEqual(e.ObjectNew.(*workv1.ManifestWork).Spec.Workload.Manifests,
				e.ObjectOld.(*workv1.ManifestWork).Spec.Workload.Manifests) {
				return true
			}

			if !maps.Equal(e.ObjectNew.GetLabels(), e.ObjectOld.GetLabels()) {
				return true
			}

			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.GetLabels()[ownerLabelKey] == ownerLabelValue
		},
	}
}

// getMchPred requires a *unstructured.Unstructured watch source.
func getMchPred(c client.Client) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// this is for operator restart, the mch CREATE event will be caught and the mch should be ready
			u, ok := e.Object.(*unstructured.Unstructured)
			if !ok {
				log.V(1).Info("MCH predicate received unexpected type in Create", "type", fmt.Sprintf("%T", e.Object))
				return false
			}
			currentVersion, desiredVersion := config.GetMCHVersions(u)

			if e.Object.GetNamespace() == config.GetMCONamespace() &&
				currentVersion != "" && desiredVersion == currentVersion {
				// only read the image manifests configmap and enqueue the request when the MCH is
				// installed/upgraded successfully
				_, found, err := config.ReadImageManifestConfigMap(
					c,
					currentVersion,
				)
				if err != nil {
					return false
				}
				return found
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ensure the event pertains to the target namespace and object type
			uNew, ok := e.ObjectNew.(*unstructured.Unstructured)
			if !ok {
				log.V(1).Info("MCH predicate received unexpected type in Update", "type", fmt.Sprintf("%T", e.ObjectNew))
				return false
			}
			currentVersion, desiredVersion := config.GetMCHVersions(uNew)

			if e.ObjectNew.GetNamespace() == config.GetMCONamespace() &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
				currentVersion != "" && desiredVersion == currentVersion {
				currentData, _, err := config.ReadImageManifestConfigMap(
					c,
					currentVersion,
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
