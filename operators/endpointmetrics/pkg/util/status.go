// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"sort"
	"time"

	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ConditionReason string

const (
	Deployed                 ConditionReason = "Deployed"
	Disabled                 ConditionReason = "Disabled"
	Degraded                 ConditionReason = "Degraded"
	NotSupported             ConditionReason = "NotSupported"
	MaxStatusConditionsCount                 = 10
)

var (
	conditions = map[ConditionReason]oav1beta1.StatusCondition{
		Deployed: {
			Type:    "Progressing",
			Reason:  string(Deployed),
			Message: "Metrics collector deployed",
			Status:  metav1.ConditionTrue,
		},
		Disabled: {
			Type:    "Disabled",
			Reason:  string(Disabled),
			Message: "enableMetrics is set to False",
			Status:  metav1.ConditionTrue,
		},
		Degraded: {
			Type:    "Degraded",
			Reason:  string(Degraded),
			Message: "Metrics collector deployment not successful",
			Status:  metav1.ConditionTrue,
		},
		NotSupported: {
			Type:    "NotSupported",
			Reason:  string(NotSupported),
			Message: "No Prometheus service found in this cluster",
			Status:  metav1.ConditionTrue,
		},
	}
)

func ReportStatus(ctx context.Context, client client.Client, conditionReason ConditionReason, addonName, addonNs string) error {
	newCondition := conditions[conditionReason]
	newCondition.LastTransitionTime = metav1.NewTime(time.Now())

	// Fetch the ObservabilityAddon instance in local cluster, and update the status
	// Retry on conflict
	obsAddon := &oav1beta1.ObservabilityAddon{}
	retryErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := client.Get(ctx, types.NamespacedName{Name: addonName, Namespace: addonNs}, obsAddon); err != nil {
			return err
		}

		if !shouldUpdateConditions(obsAddon.Status.Conditions, newCondition) {
			return nil
		}

		obsAddon.Status.Conditions = mutateOrAppend(obsAddon.Status.Conditions, newCondition)

		if len(obsAddon.Status.Conditions) > MaxStatusConditionsCount {
			obsAddon.Status.Conditions = obsAddon.Status.Conditions[len(obsAddon.Status.Conditions)-MaxStatusConditionsCount:]
		}

		return client.Status().Update(ctx, obsAddon)
	})
	if retryErr != nil {
		return retryErr
	}

	return nil
}

// mutateOrAppend updates the status conditions with the new condition.
// If the condition already exists, it updates it with the new condition.
// If the condition does not exist, it appends the new condition to the status conditions.
func mutateOrAppend(conditions []oav1beta1.StatusCondition, newCondition oav1beta1.StatusCondition) []oav1beta1.StatusCondition {
	if len(conditions) == 0 {
		return []oav1beta1.StatusCondition{newCondition}
	}

	for i, condition := range conditions {
		if condition.Type == newCondition.Type {
			// Update the existing condition
			conditions[i] = newCondition
			return conditions
		}
	}
	// If the condition type does not exist, append the new condition
	return append(conditions, newCondition)
}

// shouldAppendCondition checks if the new condition should be appended to the status conditions
// based on the last condition in the slice.
func shouldUpdateConditions(conditions []oav1beta1.StatusCondition, newCondition oav1beta1.StatusCondition) bool {
	if len(conditions) == 0 {
		return true
	}

	sort.Slice(conditions, func(i, j int) bool {
		return conditions[i].LastTransitionTime.Before(&conditions[j].LastTransitionTime)
	})

	lastCondition := conditions[len(conditions)-1]

	return lastCondition.Type != newCondition.Type ||
		lastCondition.Status != newCondition.Status ||
		lastCondition.Reason != newCondition.Reason ||
		lastCondition.Message != newCondition.Message
}
