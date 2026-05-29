// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

// PrometheusK8sRuleLabels are the conventional labels required for the OpenShift monitoring stack
// (prometheus-k8s) to select PrometheusRule resources via ruleSelector.
//
// Without these, a PrometheusRule object can exist in the cluster but never be evaluated, resulting
// in missing acm_rs:* recording-rule series (and empty dashboards).
var PrometheusK8sRuleLabels = map[string]string{
	"prometheus": "k8s",
	"role":       "alert-rules",
}
