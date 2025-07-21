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
	ClusterName              string `yaml:"cluster-name"`
	ObservatoriumAPIEndpoint string `yaml:"observatorium-api-endpoint"`
	AlertmanagerEndpoint     string `yaml:"alertmanager-endpoint"`
	AlertmanagerRouterCA     string `yaml:"alertmanager-router-ca"`
	UWMAlertingDisabled      bool   `yaml:"uwm-alerting-disabled"`
}

type RecordingRule struct {
	Record string `yaml:"record"`
	Expr   string `yaml:"expr"`
}
type CollectRule struct {
	Collect     string            `yaml:"collect"`
	Annotations map[string]string `yaml:"annotations"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for"`
	Metrics     DynamicMetrics    `yaml:"dynamic_metrics"`
}

type DynamicMetrics struct {
	NameList  []string `yaml:"names"`
	MatchList []string `yaml:"matches"`
}

type CollectRuleSelector struct {
	MatchExpression []metav1.LabelSelectorRequirement `yaml:"matchExpressions"`
}

// CollectRuleGroup structure contains information of a group of collect rules used for
// dnamically collecting metrics.
type CollectRuleGroup struct {
	Name            string              `yaml:"group"`
	Annotations     map[string]string   `yaml:"annotations"`
	Selector        CollectRuleSelector `yaml:"selector"`
	CollectRuleList []CollectRule       `yaml:"rules"`
}
type MetricsAllowlist struct {
	NameList             []string           `yaml:"names"`
	MatchList            []string           `yaml:"matches"`
	RenameMap            map[string]string  `yaml:"renames"`
	RuleList             []RecordingRule    `yaml:"rules"` //deprecated
	RecordingRuleList    []RecordingRule    `yaml:"recording_rules"`
	CollectRuleGroupList []CollectRuleGroup `yaml:"collect_rules"`
}
