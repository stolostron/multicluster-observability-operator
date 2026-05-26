// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"slices"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

var standardConditionTypes = []string{"Available", "Progressing", "Degraded"}

func updateAddonStatus(ctx context.Context, c client.Client, addonList mcov1beta1.ObservabilityAddonList) error {
	for _, addon := range addonList.Items {
		if addon.Status.Conditions == nil || len(addon.Status.Conditions) == 0 { //nolint:gosimple
			continue
		}
		obsAddonConditions := filterStandardConditions(convertConditionsToMeta(addon.Status.Conditions))
		isUpdated := false
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			managedclusteraddon := &addonv1alpha1.ManagedClusterAddOn{}
			err := c.Get(ctx, types.NamespacedName{
				Name:      util.ManagedClusterAddonName,
				Namespace: addon.ObjectMeta.Namespace,
			}, managedclusteraddon)
			if err != nil {
				if errors.IsNotFound(err) {
					log.Info("managedclusteraddon does not exist", "namespace", addon.ObjectMeta.Namespace, "name", util.ManagedClusterAddonName)
					return nil
				}
				log.Error(err, "Failed to get managedclusteraddon", "namespace", addon.ObjectMeta.Namespace, "name", util.ManagedClusterAddonName)
				return err
			}

			if equality.Semantic.DeepEqual(obsAddonConditions, managedclusteraddon.Status.Conditions) {
				return nil
			}

			managedclusteraddon.Status.Conditions = obsAddonConditions
			isUpdated = true

			return c.Status().Update(context.TODO(), managedclusteraddon)
		})
		if retryErr != nil {
			log.Error(retryErr, "Failed to update status for managedclusteraddon", "namespace", addon.ObjectMeta.Namespace)
			return retryErr
		}

		if isUpdated {
			log.Info("Updated status for managedclusteraddon", "namespace", addon.ObjectMeta.Namespace)
		}
	}

	return nil
}

func convertConditionsToMeta(conditions []mcov1beta1.StatusCondition) []metav1.Condition {
	var metaConditions []metav1.Condition
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
