// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package placementrule

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	config "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
)

func getMCHPred(c client.Client) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// this is for operator restart, the mch CREATE event will be caught and the mch should be ready
			if e.Object.GetNamespace() == config.GetMCONamespace() &&
				e.Object.(*mchv1.MultiClusterHub).Status.CurrentVersion != "" &&
				e.Object.(*mchv1.MultiClusterHub).Status.DesiredVersion == e.Object.(*mchv1.MultiClusterHub).Status.CurrentVersion {
				// only read the image manifests configmap and enqueue the request when the MCH is
				// installed/upgraded successfully
				ok, err := config.ReadImageManifestConfigMap(
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
			if e.ObjectNew.GetNamespace() == config.GetMCONamespace() &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
				e.ObjectNew.(*mchv1.MultiClusterHub).Status.CurrentVersion != "" &&
				e.ObjectNew.(*mchv1.MultiClusterHub).Status.DesiredVersion == e.ObjectNew.(*mchv1.MultiClusterHub).Status.CurrentVersion {
				// / only read the image manifests configmap and enqueue the request when the MCH is
				// installed/upgraded successfully
				ok, err := config.ReadImageManifestConfigMap(
					c,
					e.ObjectNew.(*mchv1.MultiClusterHub).Status.CurrentVersion,
				)
				if err != nil {
					return false
				}
				return ok
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}
