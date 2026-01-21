// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HubInfo is the struct that contains the common information about the hub
// cluster, for example the name of managed cluster on the hub, the URL of
// observatorium api gateway, the URL of hub alertmanager and the CA for the
// hub router.
type HubInfo struct {
	ClusterName              string `json:"cluster-name"`
	ObservatoriumAPIEndpoint string `json:"observatorium-api-endpoint"`
	AlertmanagerEndpoint     string `json:"alertmanager-endpoint"`
	AlertmanagerRouterCA     string `json:"alertmanager-router-ca"`
	UWMAlertingDisabled      bool   `json:"uwm-alerting-disabled"`
	HubClusterID             string `json:"hub-cluster-id"`
}

type RecordingRule struct {
	Record string `json:"record"`
	Expr   string `json:"expr"`
}
type CollectRule struct {
	Collect     string            `json:"collect"`
	Annotations map[string]string `json:"annotations"`
	Expr        string            `json:"expr"`
	For         string            `json:"for"`
	Metrics     DynamicMetrics    `json:"dynamic_metrics"`
}

type DynamicMetrics struct {
	NameList  []string `json:"names"`
	MatchList []string `json:"matches"`
}

type CollectRuleSelector struct {
	MatchExpression []metav1.LabelSelectorRequirement `json:"matchExpressions"`
}

// CollectRuleGroup structure contains information of a group of collect rules used for
// dnamically collecting metrics.
type CollectRuleGroup struct {
	Name            string              `json:"group"`
	Annotations     map[string]string   `json:"annotations"`
	Selector        CollectRuleSelector `json:"selector"`
	CollectRuleList []CollectRule       `json:"rules"`
}
type MetricsAllowlist struct {
	NameList             []string           `json:"names"`
	MatchList            []string           `json:"matches"`
	RenameMap            map[string]string  `json:"renames"`
	RuleList             []RecordingRule    `json:"rules"` // deprecated
	RecordingRuleList    []RecordingRule    `json:"recording_rules"`
	CollectRuleGroupList []CollectRuleGroup `json:"collect_rules"`
}
