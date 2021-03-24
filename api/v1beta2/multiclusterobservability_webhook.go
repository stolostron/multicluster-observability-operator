// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package v1beta2

import (
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// +kubebuilder:docs-gen:collapse=Go imports

var cronjoblog = logf.Log.WithName("multiClusterObservability-resource")

/*
This setup is doubles as setup for our conversion webhooks: as long as our
types implement the
[Hub](https://godoc.org/sigs.k8s.io/controller-runtime/pkg/conversion#Hub) and
[Convertible](https://godoc.org/sigs.k8s.io/controller-runtime/pkg/conversion#Convertible)
interfaces, a conversion webhook will be registered.
*/

func (mco *MultiClusterObservability) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(mco).
		Complete()
}
