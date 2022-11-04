// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package placementrule

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	config "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

func getCertSecretPred(c client.Client, ingressCtlCrdExists bool) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.ServerCACerts &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				// generate the certificate for managed cluster
				log.Info("generate managedcluster observability certificate for server certificate CREATE")
				managedClusterObsCert, _ = generateObservabilityServerCACerts(c)
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if (e.ObjectNew.GetName() == config.ServerCACerts &&
				e.ObjectNew.GetNamespace() == config.GetDefaultNamespace()) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				// regenerate the certificate for managed cluster
				log.Info("generate managedcluster observability certificate for server certificate UPDATE")
				managedClusterObsCert, _ = generateObservabilityServerCACerts(c)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}
