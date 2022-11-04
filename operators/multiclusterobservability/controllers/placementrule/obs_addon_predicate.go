package placementrule

import (
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
)

func getObsAddonPred(c client.Client, ingressCtlCrdExists bool) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == obsAddonName &&
				e.ObjectNew.GetLabels()[ownerLabelKey] == ownerLabelValue &&
				!reflect.DeepEqual(e.ObjectNew.(*mcov1beta1.ObservabilityAddon).Status.Conditions,
					e.ObjectOld.(*mcov1beta1.ObservabilityAddon).Status.Conditions) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == obsAddonName &&
				e.Object.GetLabels()[ownerLabelKey] == ownerLabelValue {
				log.Info(
					"DeleteFunc",
					"obsAddonNamespace",
					e.Object.GetNamespace(),
					"obsAddonName",
					e.Object.GetName(),
				)
				/* #nosec */
				removePostponeDeleteAnnotationForManifestwork(c, e.Object.GetNamespace())
				return true
			}
			return false
		},
	}
}
