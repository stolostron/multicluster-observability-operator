// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"errors"
	"slices"

	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var standardConditionTypes = []string{"Available", "Progressing", "Degraded"}

func updateAddonStatus(ctx context.Context, c client.Client, addonList mcov1beta1.ObservabilityAddonList) error {
	var allErrors []error
	for _, addon := range addonList.Items {
		if len(addon.Status.Conditions) == 0 {
			continue
		}
		obsAddonConditions := filterStandardConditions(convertConditionsToMeta(addon.Status.Conditions))
		isUpdated := false
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			managedclusteraddon := &addonv1alpha1.ManagedClusterAddOn{}
			err := c.Get(ctx, types.NamespacedName{
				Name:      config.ManagedClusterAddonName,
				Namespace: addon.ObjectMeta.Namespace,
			}, managedclusteraddon)
			if err != nil {
				if apierrors.IsNotFound(err) {
					log.Info("managedclusteraddon does not exist", "namespace", addon.ObjectMeta.Namespace, "name", config.ManagedClusterAddonName)
					return nil
				}
				log.Error(err, "Failed to get managedclusteraddon", "namespace", addon.ObjectMeta.Namespace, "name", config.ManagedClusterAddonName)
				return err
			}

			desiredAddon := managedclusteraddon.DeepCopy()
			for _, cond := range obsAddonConditions {
				if meta.IsStatusConditionPresentAndEqual(desiredAddon.Status.Conditions, cond.Type, cond.Status) {
					continue
				}
				if meta.SetStatusCondition(&desiredAddon.Status.Conditions, cond) {
					isUpdated = true
				}
			}

			if !isUpdated {
				return nil
			}

			return c.Status().Patch(ctx, desiredAddon, client.MergeFrom(managedclusteraddon))
		})
		if retryErr != nil {
			log.Error(retryErr, "Failed to update status for managedclusteraddon", "namespace", addon.ObjectMeta.Namespace)
			allErrors = append(allErrors, retryErr)
		}

		if retryErr == nil && isUpdated {
			log.Info("Updated status for managedclusteraddon", "namespace", addon.ObjectMeta.Namespace)
		}
	}

	if len(allErrors) > 0 {
		return errors.Join(allErrors...)
	}

	return nil
}

func convertConditionsToMeta(conditions []mcov1beta1.StatusCondition) []metav1.Condition {
	metaConditions := make([]metav1.Condition, 0, len(conditions))
	for _, c := range conditions {
		metaCondition := metav1.Condition{
			Type:               c.Type,
			Status:             c.Status,
			LastTransitionTime: c.LastTransitionTime,
			Reason:             c.Reason,
			Message:            c.Message,
		}
		metaConditions = append(metaConditions, metaCondition)
	}
	return metaConditions
}

func filterStandardConditions(conditions []metav1.Condition) []metav1.Condition {
	ret := make([]metav1.Condition, 0, len(conditions))
	for _, c := range conditions {
		if slices.Contains(standardConditionTypes, c.Type) {
			ret = append(ret, c)
		}
	}

	return ret
}
