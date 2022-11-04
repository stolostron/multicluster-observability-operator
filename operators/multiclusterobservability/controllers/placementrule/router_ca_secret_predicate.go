// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package placementrule

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	config "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

func getRouterCASecretPred(c client.Client, ingressCtlCrdExists bool) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if (e.Object.GetNamespace() == config.OpenshiftIngressOperatorNamespace &&
				e.Object.GetName() == config.OpenshiftIngressRouteCAName) ||
				(e.Object.GetNamespace() == config.OpenshiftIngressNamespace &&
					e.Object.GetName() == config.OpenshiftIngressDefaultCertName) {
				// generate the hubInfo secret
				hubInfoSecret, _ = generateHubInfoSecret(
					c,
					config.GetDefaultNamespace(),
					spokeNameSpace,
					ingressCtlCrdExists,
				)
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if ((e.ObjectNew.GetNamespace() == config.OpenshiftIngressOperatorNamespace &&
				e.ObjectNew.GetName() == config.OpenshiftIngressRouteCAName) ||
				(e.ObjectNew.GetNamespace() == config.OpenshiftIngressNamespace &&
					e.ObjectNew.GetName() == config.OpenshiftIngressDefaultCertName)) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				// regenerate the hubInfo secret
				hubInfoSecret, _ = generateHubInfoSecret(
					c,
					config.GetDefaultNamespace(),
					spokeNameSpace,
					ingressCtlCrdExists,
				)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}
