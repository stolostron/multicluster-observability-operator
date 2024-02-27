// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"time"

	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	conditions = map[string]map[string]string{
		"Deployed": {
			"type":    "Progressing",
			"reason":  "Deployed",
			"message": "Metrics collector deployed"},
		"Disabled": {
			"type":    "Disabled",
			"reason":  "Disabled",
			"message": "enableMetrics is set to False"},
		"Degraded": {
			"type":    "Degraded",
			"reason":  "Degraded",
			"message": "Metrics collector deployment not successful"},
		"NotSupported": {
			"type":    "NotSupported",
			"reason":  "NotSupported",
			"message": "No Prometheus service found in this cluster"},
	}
)

func ReportStatus(ctx context.Context, client client.Client, i *oav1beta1.ObservabilityAddon, t string, reportStatus bool) {
	if !reportStatus {
		return
	}
	i.Status.Conditions = []oav1beta1.StatusCondition{
		{
			Type:               conditions[t]["type"],
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             conditions[t]["reason"],
			Message:            conditions[t]["message"],
		},
	}
	err := client.Status().Update(ctx, i)
	if err != nil {
		log.Error(err, "Failed to update status for observabilityaddon")
	}
}
