// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"fmt"
	"strings"
)

// BuildNamespaceFilter creates a namespace filter string for Prometheus queries
func BuildNamespaceFilter(nsConfig RSPrometheusRuleConfig) (string, error) {
	ns := nsConfig.NamespaceFilterCriteria
	if len(ns.InclusionCriteria) > 0 && len(ns.ExclusionCriteria) > 0 {
		return "", fmt.Errorf("only one of inclusion or exclusion criteria allowed for namespacefiltercriteria")
	}
	if len(ns.InclusionCriteria) > 0 {
		return fmt.Sprintf(`namespace=~"%s"`, strings.Join(ns.InclusionCriteria, "|")), nil
	}
	if len(ns.ExclusionCriteria) > 0 {
		return fmt.Sprintf(`namespace!~"%s"`, strings.Join(ns.ExclusionCriteria, "|")), nil
	}
	return `namespace!=""`, nil
}

// BuildLabelJoin creates a label join string for Prometheus queries
func BuildLabelJoin(labelFilters []RSLabelFilter) (string, error) {
	for _, l := range labelFilters {
		if l.LabelName != "label_env" {
			continue
		}
		if len(l.InclusionCriteria) > 0 && len(l.ExclusionCriteria) > 0 {
			return "", fmt.Errorf("only one of inclusion or exclusion allowed for label_env")
		}
		var selector string
		if len(l.InclusionCriteria) > 0 {
			selector = fmt.Sprintf(`kube_namespace_labels{label_env=~"%s"}`, strings.Join(l.InclusionCriteria, "|"))
		} else if len(l.ExclusionCriteria) > 0 {
			selector = fmt.Sprintf(`kube_namespace_labels{label_env!~"%s"}`, strings.Join(l.ExclusionCriteria, "|"))
		} else {
			continue
		}
		return fmt.Sprintf(`* on (namespace) group_left() (%s or kube_namespace_labels{label_env=""})`, selector), nil
	}
	return "", nil
}
