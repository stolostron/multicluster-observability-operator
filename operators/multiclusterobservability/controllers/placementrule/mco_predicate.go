// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package placementrule

import (
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	config "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

func getMCOPred(c client.Client, ingressCtlCrdExists bool) predicate.Funcs {
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
			var err error = nil
			hubInfoSecret, err = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, ingressCtlCrdExists)
			if err != nil {
				log.Error(err, "unable to get HubInfoSecret", "controller", "PlacementRule")
			}
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			retval := false
			mco := e.ObjectNew.(*mcov1beta2.MultiClusterObservability)
			oldAlertingStatus := config.IsAlertingDisabled()
			newAlertingStatus := config.IsAlertingDisabledInSpec(mco)
			// if value changed, then mustReconcile is true
			if oldAlertingStatus != newAlertingStatus {
				config.SetAlertingDisabled(newAlertingStatus)
				var err error = nil

				hubInfoSecret, err = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, ingressCtlCrdExists)
				if err != nil {
					log.Error(err, "unable to get HubInfoSecret", "controller", "PlacementRule")
				}

				retval = true
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
