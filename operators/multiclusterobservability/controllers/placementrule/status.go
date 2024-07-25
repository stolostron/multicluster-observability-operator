// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"reflect"
	"slices"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

var (
	stdConditions = []string{"Available", "Progressing", "Degraded"}
)

func updateAddonStatus(ctx context.Context, c client.Client, addonList mcov1beta1.ObservabilityAddonList) error {
	for _, addon := range addonList.Items {
		if addon.Status.Conditions == nil || len(addon.Status.Conditions) == 0 {
			continue
		}
		obsAddonStdConditions := filterStandardConditions(convertConditionsToMeta(addon.Status.Conditions))

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

			sortConditionsFunc := func(a, b metav1.Condition) int {
				if a.Type < b.Type {
					return -1
				}
				if a.Type > b.Type {
					return 1
				}
				return 0
			}

			clusterAddonStdConditions := filterStandardConditions(managedclusteraddon.Status.Conditions)
			slices.SortFunc(clusterAddonStdConditions, sortConditionsFunc)
			slices.SortFunc(obsAddonStdConditions, sortConditionsFunc)

			if reflect.DeepEqual(clusterAddonStdConditions, obsAddonStdConditions) {
				return nil
			}

			newClusterAddonConditions := make([]metav1.Condition, len(obsAddonStdConditions))
			copy(newClusterAddonConditions, obsAddonStdConditions)
			for _, clusterCondition := range managedclusteraddon.Status.Conditions {
				if !slices.Contains(stdConditions, clusterCondition.Type) {
					newClusterAddonConditions = append(newClusterAddonConditions, clusterCondition)
				}
			}

			managedclusteraddon.Status.Conditions = newClusterAddonConditions
			log.Info("Updating status for managedclusteraddon", "namespace", addon.ObjectMeta.Namespace)

			return c.Status().Update(context.TODO(), managedclusteraddon)
		})
		if retryErr != nil {
			log.Error(retryErr, "Failed to update status for managedclusteraddon", "namespace", addon.ObjectMeta.Namespace)
			return retryErr
		}

		return nil
	}

	return nil
}

func filterStandardConditions(conditions []metav1.Condition) []metav1.Condition {
	var standardConditions []metav1.Condition
	for _, condition := range conditions {
		if slices.Contains(stdConditions, condition.Type) {
			standardConditions = append(standardConditions, condition)
		}
	}
	return standardConditions
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
