// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"time"

	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type StatusConditionName string

const (
	DeployedStatus           StatusConditionName = "Deployed"
	DisabledStatus           StatusConditionName = "Disabled"
	DegradedStatus           StatusConditionName = "Degraded"
	NotSupportedStatus       StatusConditionName = "NotSupported"
	MaxStatusConditionsCount                     = 10
)

var (
	conditions = map[StatusConditionName]*oav1beta1.StatusCondition{
		DeployedStatus: {
			Type:    "Progressing",
			Reason:  "Deployed",
			Message: "Metrics collector deployed",
			Status:  metav1.ConditionTrue,
		},
		DisabledStatus: {
			Type:    "Disabled",
			Reason:  "Disabled",
			Message: "enableMetrics is set to False",
			Status:  metav1.ConditionTrue,
		},
		DegradedStatus: {
			Type:    "Degraded",
			Reason:  "Degraded",
			Message: "Metrics collector deployment not successful",
			Status:  metav1.ConditionTrue,
		},
		NotSupportedStatus: {
			Type:    "NotSupported",
			Reason:  "NotSupported",
			Message: "No Prometheus service found in this cluster",
			Status:  metav1.ConditionTrue,
		},
	}
)

func ReportStatus(ctx context.Context, client client.Client, condition StatusConditionName, addonName, addonNs string) {
	newCondition := conditions[condition].DeepCopy()
	newCondition.LastTransitionTime = metav1.NewTime(time.Now())

	// Fetch the ObservabilityAddon instance in local cluster, and update the status
	// Retry on conflict
	obsAddon := &oav1beta1.ObservabilityAddon{}
	retryErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := client.Get(ctx, types.NamespacedName{Name: addonName, Namespace: addonNs}, obsAddon); err != nil {
			return err
		}

		if !shouldAppendCondition(obsAddon.Status.Conditions, newCondition) {
			return nil
		}

		obsAddon.Status.Conditions = append(obsAddon.Status.Conditions, *newCondition)

		if len(obsAddon.Status.Conditions) > MaxStatusConditionsCount {
			obsAddon.Status.Conditions = obsAddon.Status.Conditions[len(obsAddon.Status.Conditions)-MaxStatusConditionsCount:]
		}

		return client.Status().Update(ctx, obsAddon)
	})
	if retryErr != nil {
		log.Error(retryErr, "Failed to update status for observabilityaddon")
	}
}

// shouldAppendCondition checks if the new condition should be appended to the status conditions
// based on the last condition in the slice.
func shouldAppendCondition(conditions []oav1beta1.StatusCondition, newCondition *oav1beta1.StatusCondition) bool {
	if len(conditions) == 0 {
		return true
	}

	lastCondition := conditions[len(conditions)-1]

	return lastCondition.Type != newCondition.Type ||
		lastCondition.Status != newCondition.Status ||
		lastCondition.Reason != newCondition.Reason ||
		lastCondition.Message != newCondition.Message
}
