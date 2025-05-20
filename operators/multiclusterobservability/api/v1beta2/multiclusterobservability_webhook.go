// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package v1beta2

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:docs-gen:collapse=Go imports

// log is for logging in this package.
var multiclusterobservabilitylog = logf.Log.WithName("multiclusterobservability-resource")

var kubeClient kubernetes.Interface

func (mco *MultiClusterObservability) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		WithValidator(&MultiClusterObservability{}).
		For(mco).
		Complete()
}

// +kubebuilder:webhook:path=/validate-observability-open-cluster-management-io-v1beta2-multiclusterobservability,mutating=false,failurePolicy=fail,sideEffects=None,groups=observability.open-cluster-management.io,resources=multiclusterobservabilities,verbs=create;update,versions=v1beta2,name=vmulticlusterobservability.observability.open-cluster-management.io,admissionReviewVersions={v1}

var _ webhook.CustomValidator = &MultiClusterObservability{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (mco *MultiClusterObservability) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	multiclusterobservabilitylog.Info("validate create", "name", mco.Name)
	return nil, mco.validateMultiClusterObservability(nil)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (mco *MultiClusterObservability) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	multiclusterobservabilitylog.Info("validate update", "name", mco.Name)
	return nil, mco.validateMultiClusterObservability(oldObj)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (mco *MultiClusterObservability) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	multiclusterobservabilitylog.Info("validate delete", "name", mco.Name)

	// no validation logic upon object delete.
	return nil, nil
}

// validateMultiClusterObservability validates  the name and the spec of the MultiClusterObservability CR.
func (mco *MultiClusterObservability) validateMultiClusterObservability(old runtime.Object) error {
	var allErrs field.ErrorList
	if err := mco.validateMultiClusterObservabilityName(); err != nil {
		allErrs = append(allErrs, err)
	}
	if err := mco.validateMultiClusterObservabilitySpec(); err != nil {
		allErrs = append(allErrs, err)
	}

	// validate the MultiClusterObservability CR update
	if old != nil {
		if errlists := mco.validateUpdateMultiClusterObservabilitySpec(old); errlists != nil {
			allErrs = append(allErrs, errlists...)
		}
	}
	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "observability.open-cluster-management.io", Kind: "MultiClusterObservability"},
		mco.Name, allErrs)
}

// validateMultiClusterObservabilityName validates the name of the MultiClusterObservability CR.
// Validating the length of a string field can be done declaratively by the validation schema.
// But the `ObjectMeta.Name` field is defined in a shared package under the apimachinery repo,
// so we can't declaratively validate it using the validation schema.
func (mco *MultiClusterObservability) validateMultiClusterObservabilityName() *field.Error {
	return nil
}

// validateMultiClusterObservabilitySpec validates the spec of the MultiClusterObservability CR.
// notice that some fields are declaratively validated by OpenAPI schema with `// +kubebuilder:validation` in the type
// definition.
func (mco *MultiClusterObservability) validateMultiClusterObservabilitySpec() *field.Error {
	// The field helpers from the kubernetes API machinery help us return nicely structured validation errors.
	return nil
}

// validateUpdateMultiClusterObservabilitySpec validates the update of the MultiClusterObservability CR.
func (mco *MultiClusterObservability) validateUpdateMultiClusterObservabilitySpec(old runtime.Object) field.ErrorList {
	return mco.validateUpdateMultiClusterObservabilityStorageSize(old)
}

// validateUpdateMultiClusterObservabilityStorageSize validates the update of storage size in the
// MultiClusterObservability CR.
func (mco *MultiClusterObservability) validateUpdateMultiClusterObservabilityStorageSize(
	old runtime.Object,
) field.ErrorList {
	var errs field.ErrorList
	oldMCO := old.(*MultiClusterObservability)
	kubeClient, err := createOrGetKubeClient()
	if err != nil {
		return append(errs, field.InternalError(nil, err))
	}

	selectedSC, err := getSelectedStorageClassForMultiClusterObservability(kubeClient, oldMCO)
	if err != nil {
		return append(errs, field.InternalError(nil, err))
	}

	selectedSCAllowResize, err := storageClassAllowVolumeExpansion(kubeClient, selectedSC)
	if err != nil {
		return append(errs, field.InternalError(nil, err))
	}

	// if the selected storage class is allowed resize, then return with no error
	if selectedSCAllowResize {
		return nil
	}

	mcoOldConfig := oldMCO.Spec.StorageConfig
	mcoNewConfig := mco.Spec.StorageConfig
	if mcoOldConfig != nil && mcoNewConfig != nil {
		storageConfigFieldPath := field.NewPath("spec").Child("storageConfig")
		storageForbiddenResize := "is forbidden to update."
		if mcoOldConfig.AlertmanagerStorageSize != mcoNewConfig.AlertmanagerStorageSize {
			errs = append(
				errs,
				field.Forbidden(storageConfigFieldPath.Child("alertmanagerStorageSize"), storageForbiddenResize),
			)
		}
		if mcoOldConfig.CompactStorageSize != mcoNewConfig.CompactStorageSize {
			errs = append(
				errs,
				field.Forbidden(storageConfigFieldPath.Child("compactStorageSize"), storageForbiddenResize),
			)
		}
		if mcoOldConfig.ReceiveStorageSize != mcoNewConfig.ReceiveStorageSize {
			errs = append(
				errs,
				field.Forbidden(storageConfigFieldPath.Child("receiveStorageSize"), storageForbiddenResize),
			)
		}
		if mcoOldConfig.StoreStorageSize != mcoNewConfig.StoreStorageSize {
			errs = append(
				errs,
				field.Forbidden(storageConfigFieldPath.Child("storeStorageSize"), storageForbiddenResize),
			)
		}
		if mcoOldConfig.RuleStorageSize != mcoNewConfig.RuleStorageSize {
			errs = append(
				errs,
				field.Forbidden(storageConfigFieldPath.Child("ruleStorageSize"), storageForbiddenResize),
			)
		}
		return errs
	}

	return nil
}

// createOrGetKubeClient creates or gets the existing kubeClient.
func createOrGetKubeClient() (kubernetes.Interface, error) {
	if kubeClient != nil {
		return kubeClient, nil
	}
	kubeClient, err := kubernetes.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		return nil, err
	}
	return kubeClient, nil
}

// getSelectedStorageClassForMultiClusterObservability get secected for the MultiClusterObservability CR.
func getSelectedStorageClassForMultiClusterObservability(
	c kubernetes.Interface,
	mco *MultiClusterObservability,
) (string, error) {
	scInCR := ""
	if mco.Spec.StorageConfig != nil {
		scInCR = mco.Spec.StorageConfig.StorageClass
	}

	scList, err := c.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	scMatch := false
	defaultSC := ""
	for _, sc := range scList.Items {
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			defaultSC = sc.Name
		}
		if sc.Name == scInCR {
			scMatch = true
		}
	}
	expectedSC := defaultSC
	if scMatch {
		expectedSC = scInCR
	}

	if expectedSC == "" {
		multiclusterobservabilitylog.Info("The storageclass specified in MCO CR is not available, also there is no default storageclass")
	}

	return expectedSC, nil
}

// storageClassAllowVolumeExpansion check if the storageclass allow volume expansion.
func storageClassAllowVolumeExpansion(c kubernetes.Interface, name string) (bool, error) {
	sc, err := c.StorageV1().StorageClasses().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	scAllowVolumeExpansion := false
	// AllowVolumeExpansion may be omitted with default false value.
	if sc.AllowVolumeExpansion != nil {
		scAllowVolumeExpansion = *sc.AllowVolumeExpansion
	}

	return scAllowVolumeExpansion, nil
}
