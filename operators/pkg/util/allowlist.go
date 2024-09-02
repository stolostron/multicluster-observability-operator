// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"strings"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
)

func GetAllowList(client client.Client, name, namespace string) (*operatorconfig.MetricsAllowlist,
	*operatorconfig.MetricsAllowlist, *operatorconfig.MetricsAllowlist, error) {
	found := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	err := client.Get(context.TODO(), namespacedName, found)
	if err != nil {
		return nil, nil, nil, err
	}

	return ParseAllowlistConfigMap(*found)
}

func ParseAllowlistConfigMap(cm corev1.ConfigMap) (*operatorconfig.MetricsAllowlist,
	*operatorconfig.MetricsAllowlist, *operatorconfig.MetricsAllowlist, error) {
	allowlist := &operatorconfig.MetricsAllowlist{}
	err := yaml.Unmarshal([]byte(cm.Data["metrics_list.yaml"]), allowlist)
	if err != nil {
		log.Error(err, "Failed to unmarshal metrics_list.yaml data in configmap ",
			"namespace", cm.ObjectMeta.Namespace, "name", cm.ObjectMeta.Name)
		return nil, nil, nil, err
	}
	ocp3Allowlist := &operatorconfig.MetricsAllowlist{}
	err = yaml.Unmarshal([]byte(cm.Data["ocp311_metrics_list.yaml"]), ocp3Allowlist)
	if err != nil {
		log.Error(err, "Failed to unmarshal ocp311_metrics_list data in configmap ",
			"namespace", cm.ObjectMeta.Namespace, "name", cm.ObjectMeta.Name)
		return nil, nil, nil, err
	}
	uwlAllowlist := &operatorconfig.MetricsAllowlist{}
	err = yaml.Unmarshal([]byte(cm.Data["uwl_metrics_list.yaml"]), uwlAllowlist)
	if err != nil {
		log.Error(err, "Failed to unmarshal uwl_metrics_list data in configmap ",
			"namespace", cm.ObjectMeta.Namespace, "name", cm.ObjectMeta.Name)
		return nil, nil, nil, err
	}
	return allowlist, ocp3Allowlist, uwlAllowlist, nil
}

func MergeAllowlist(allowlist, customAllowlist, ocp3Allowlist, uwlAllowlist,
	customUwlAllowlist *operatorconfig.MetricsAllowlist) (*operatorconfig.MetricsAllowlist,
	*operatorconfig.MetricsAllowlist, *operatorconfig.MetricsAllowlist) {
	allowlist.NameList = mergeMetrics(allowlist.NameList, customAllowlist.NameList)
	allowlist.MatchList = mergeMetrics(allowlist.MatchList, customAllowlist.MatchList)
	allowlist.CollectRuleGroupList = mergeCollectorRuleGroupList(allowlist.CollectRuleGroupList,
		customAllowlist.CollectRuleGroupList)
	if customAllowlist.RecordingRuleList != nil {
		allowlist.RecordingRuleList = append(allowlist.RecordingRuleList, customAllowlist.RecordingRuleList...)
	} else {
		//check if rules are specified for backward compatibility
		allowlist.RecordingRuleList = append(allowlist.RecordingRuleList, customAllowlist.RuleList...)
	}
	if customAllowlist.RenameMap == nil {
		customAllowlist.RenameMap = make(map[string]string)
	}
	for k, v := range customAllowlist.RenameMap {
		allowlist.RenameMap[k] = v
	}
	if ocp3Allowlist != nil {
		ocp3Allowlist.NameList = mergeMetrics(ocp3Allowlist.NameList, customAllowlist.NameList)
		ocp3Allowlist.MatchList = mergeMetrics(ocp3Allowlist.MatchList, customAllowlist.MatchList)
		ocp3Allowlist.RuleList = append(ocp3Allowlist.RuleList, customAllowlist.RuleList...)
		if ocp3Allowlist.RenameMap == nil {
			ocp3Allowlist.RenameMap = make(map[string]string)
		}
		for k, v := range customAllowlist.RenameMap {
			ocp3Allowlist.RenameMap[k] = v
		}
	}
	uwlAllowlist.NameList = mergeMetrics(uwlAllowlist.NameList, customUwlAllowlist.NameList)
	uwlAllowlist.MatchList = mergeMetrics(uwlAllowlist.MatchList, customUwlAllowlist.MatchList)
	uwlAllowlist.RuleList = append(uwlAllowlist.RuleList, customUwlAllowlist.RuleList...)
	if uwlAllowlist.RenameMap == nil {
		uwlAllowlist.RenameMap = make(map[string]string)
	}
	for k, v := range customUwlAllowlist.RenameMap {
		uwlAllowlist.RenameMap[k] = v
	}

	return allowlist, ocp3Allowlist, uwlAllowlist
}

func mergeMetrics(defaultAllowlist []string, customAllowlist []string) []string {
	customMetrics := []string{}
	deletedMetrics := map[string]bool{}
	for _, name := range customAllowlist {
		if !strings.HasPrefix(name, "-") {
			customMetrics = append(customMetrics, name)
		} else {
			deletedMetrics[strings.TrimPrefix(name, "-")] = true
		}
	}

	metricsRecorder := map[string]bool{}
	mergedMetrics := []string{}
	defaultAllowlist = append(defaultAllowlist, customMetrics...)
	for _, name := range defaultAllowlist {
		if metricsRecorder[name] {
			continue
		}

		if !deletedMetrics[name] {
			mergedMetrics = append(mergedMetrics, name)
			metricsRecorder[name] = true
		}
	}

	return mergedMetrics
}

func mergeCollectorRuleGroupList(defaultCollectRuleGroupList []operatorconfig.CollectRuleGroup,
	customCollectRuleGroupList []operatorconfig.CollectRuleGroup) []operatorconfig.CollectRuleGroup {
	deletedCollectRuleGroups := map[string]bool{}
	mergedCollectRuleGroups := []operatorconfig.CollectRuleGroup{}

	for _, collectRuleGroup := range customCollectRuleGroupList {
		if strings.HasPrefix(collectRuleGroup.Name, "-") {
			deletedCollectRuleGroups[strings.TrimPrefix(collectRuleGroup.Name, "-")] = true
		} else {
			mergedCollectRuleGroups = append(mergedCollectRuleGroups, collectRuleGroup)
		}
	}

	for _, collectRuleGroup := range defaultCollectRuleGroupList {
		if !deletedCollectRuleGroups[collectRuleGroup.Name] {
			mergedCollectRuleGroups = append(mergedCollectRuleGroups, collectRuleGroup)
		}
	}

	return mergedCollectRuleGroups
}
