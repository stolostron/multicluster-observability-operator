// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	config "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

func getMCOPred(c client.Client, crdMap map[string]bool) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// generate the image pull secret
			pullSecret, _ = generatePullSecret(
				c,
				config.GetImagePullSecret(e.Object.(*mcov1beta2.MultiClusterObservability).Spec),
			)

			mco := e.Object.(*mcov1beta2.MultiClusterObservability)
			alertingStatus := config.IsAlertingDisabledInSpec(mco)
			config.SetAlertingDisabled(alertingStatus)
			var err error
			hubInfoSecret, err = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, crdMap, config.IsUWMAlertingDisabledInSpec(mco))
			if err != nil {
				log.Error(err, "unable to get HubInfoSecret", "controller", "PlacementRule")
			}
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			retval := false
			updateHubInfo := false
			newMCO := e.ObjectNew.(*mcov1beta2.MultiClusterObservability)
			oldMCO := e.ObjectOld.(*mcov1beta2.MultiClusterObservability)
			oldAlertingStatus := config.IsAlertingDisabled()
			newAlertingStatus := config.IsAlertingDisabledInSpec(newMCO)

			if !reflect.DeepEqual(newMCO.Spec.AdvancedConfig, oldMCO.Spec.AdvancedConfig) {
				updateHubInfo = true
				retval = true
			}
			// if value changed, then mustReconcile is true
			if oldAlertingStatus != newAlertingStatus {
				config.SetAlertingDisabled(newAlertingStatus)
				retval = true
				updateHubInfo = true
			}

			// Check if UWM alerting status changed
			oldUWMAlertingStatus := config.IsUWMAlertingDisabledInSpec(oldMCO)
			newUWMAlertingStatus := config.IsUWMAlertingDisabledInSpec(newMCO)
			if oldUWMAlertingStatus != newUWMAlertingStatus {
				retval = true
				updateHubInfo = true
			}

			if updateHubInfo {
				var err error
				hubInfoSecret, err = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, crdMap, config.IsUWMAlertingDisabledInSpec(newMCO))
				if err != nil {
					log.Error(err, "unable to get HubInfoSecret", "controller", "PlacementRule")
				}
			}

			// only reconcile when ObservabilityAddonSpec updated
			if e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
				!reflect.DeepEqual(e.ObjectNew.(*mcov1beta2.MultiClusterObservability).Spec.ObservabilityAddonSpec,
					e.ObjectOld.(*mcov1beta2.MultiClusterObservability).Spec.ObservabilityAddonSpec) {
				if e.ObjectNew.(*mcov1beta2.MultiClusterObservability).Spec.ImagePullSecret != e.ObjectOld.(*mcov1beta2.MultiClusterObservability).Spec.ImagePullSecret {
					// regenerate the image pull secret
					pullSecret, _ = generatePullSecret(
						c,
						config.GetImagePullSecret(e.ObjectNew.(*mcov1beta2.MultiClusterObservability).Spec),
					)
				}
				retval = true
			}
			return retval
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}
}
