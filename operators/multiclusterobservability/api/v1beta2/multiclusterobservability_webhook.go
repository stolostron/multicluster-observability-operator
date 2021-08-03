// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package v1beta2

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// +kubebuilder:docs-gen:collapse=Go imports

// log is for logging in this package.
var multiclusterobservabilitylog = logf.Log.WithName("multiclusterobservability-resource")

func (mco *MultiClusterObservability) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(mco).
		Complete()
}

// +kubebuilder:webhook:path=/validate,mutating=false,failurePolicy=fail,sideEffects=None,groups=observability.open-cluster-management.io,resources=multiclusterobservabilities,verbs=create;update,versions=v1beta2,name=vmulticlusterobservability.observability.open-cluster-management.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &MultiClusterObservability{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (mco *MultiClusterObservability) ValidateCreate() error {
	multiclusterobservabilitylog.Info("validate create", "name", mco.Name)

	// TODO(morvencao): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (mco *MultiClusterObservability) ValidateUpdate(old runtime.Object) error {
	multiclusterobservabilitylog.Info("validate update", "name", mco.Name)

	// TODO(morvencao): fill in your validation logic upon object update.
	return fmt.Errorf("testing validation webhook")
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (mco *MultiClusterObservability) ValidateDelete() error {
	multiclusterobservabilitylog.Info("validate delete", "name", mco.Name)

	// TODO(morvencao): fill in your validation logic upon object delete.
	return nil
}
