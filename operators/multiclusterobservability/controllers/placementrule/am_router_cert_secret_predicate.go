// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package placementrule

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	config "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

func getAlertManagerRouterCertSecretPred(c client.Client, ingressCtlCrdExists bool) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() &&
				(e.Object.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.Object.GetName() == config.AlertmanagerRouteBYOCERTName) {
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
			if e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
				(e.ObjectNew.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.ObjectNew.GetName() == config.AlertmanagerRouteBYOCERTName) {
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
			if e.Object.GetNamespace() == config.GetDefaultNamespace() &&
				(e.Object.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.Object.GetName() == config.AlertmanagerRouteBYOCERTName) {
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
	}
}
