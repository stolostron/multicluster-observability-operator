// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"maps"
	"strings"

	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func GetAllowList(client client.Client, name, namespace string) (*operatorconfig.MetricsAllowlist,
	*operatorconfig.MetricsAllowlist, error,
) {
	found := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	err := client.Get(context.TODO(), namespacedName, found)
	if err != nil {
		return nil, nil, err
	}

	return ParseAllowlistConfigMap(*found)
}

func ParseAllowlistConfigMap(cm corev1.ConfigMap) (*operatorconfig.MetricsAllowlist,
	*operatorconfig.MetricsAllowlist, error,
) {
	allowlist := &operatorconfig.MetricsAllowlist{}
	err := yaml.Unmarshal([]byte(cm.Data["metrics_list.yaml"]), allowlist)
	if err != nil {
		log.Error(err, "Failed to unmarshal metrics_list.yaml data in configmap ",
			"namespace", cm.Namespace, "name", cm.Name)
		return nil, nil, err
	}
	uwlAllowlist := &operatorconfig.MetricsAllowlist{}
	err = yaml.Unmarshal([]byte(cm.Data["uwl_metrics_list.yaml"]), uwlAllowlist)
	if err != nil {
		log.Error(err, "Failed to unmarshal uwl_metrics_list data in configmap ",
			"namespace", cm.Namespace, "name", cm.Name)
		return nil, nil, err
	}
	return allowlist, uwlAllowlist, nil
}

func MergeAllowlist(allowlist, customAllowlist, uwlAllowlist,
	customUwlAllowlist *operatorconfig.MetricsAllowlist) (*operatorconfig.MetricsAllowlist,
	*operatorconfig.MetricsAllowlist,
) {
	allowlist.NameList = mergeMetrics(allowlist.NameList, customAllowlist.NameList)
	allowlist.MatchList = mergeMetrics(allowlist.MatchList, customAllowlist.MatchList)
	allowlist.CollectRuleGroupList = mergeCollectorRuleGroupList(allowlist.CollectRuleGroupList,
		customAllowlist.CollectRuleGroupList)
	if customAllowlist.RecordingRuleList != nil {
		allowlist.RecordingRuleList = append(allowlist.RecordingRuleList, customAllowlist.RecordingRuleList...)
	} else {
		// check if rules are specified for backward compatibility
		allowlist.RecordingRuleList = append(allowlist.RecordingRuleList, customAllowlist.RuleList...)
	}
	if allowlist.RenameMap == nil {
		allowlist.RenameMap = make(map[string]string)
	}
	maps.Copy(allowlist.RenameMap, customAllowlist.RenameMap)
	uwlAllowlist.NameList = mergeMetrics(uwlAllowlist.NameList, customUwlAllowlist.NameList)
	uwlAllowlist.MatchList = mergeMetrics(uwlAllowlist.MatchList, customUwlAllowlist.MatchList)
	uwlAllowlist.RuleList = append(uwlAllowlist.RuleList, customUwlAllowlist.RuleList...)
	uwlAllowlist.RecordingRuleList = append(uwlAllowlist.RecordingRuleList, customUwlAllowlist.RecordingRuleList...)
	if uwlAllowlist.RenameMap == nil {
		uwlAllowlist.RenameMap = make(map[string]string)
	}
	maps.Copy(uwlAllowlist.RenameMap, customUwlAllowlist.RenameMap)

	return allowlist, uwlAllowlist
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
	customCollectRuleGroupList []operatorconfig.CollectRuleGroup,
) []operatorconfig.CollectRuleGroup {
	deletedCollectRuleGroups := map[string]bool{}
	mergedCollectRuleGroups := []operatorconfig.CollectRuleGroup{}

	for _, collectRuleGroup := range customCollectRuleGroupList {
		if after, ok := strings.CutPrefix(collectRuleGroup.Name, "-"); ok {
			deletedCollectRuleGroups[after] = true
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
