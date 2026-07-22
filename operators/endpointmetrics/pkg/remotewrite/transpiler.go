// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package remotewrite

import (
	"cmp"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"k8s.io/utils/ptr"
)

func Transpile(scrapeConfig *monitoringv1alpha1.ScrapeConfig, agent *monitoringv1alpha1.PrometheusAgent) ([]*monitoringv1.RemoteWriteSpec, error) {
	if scrapeConfig == nil {
		return nil, nil
	}

	matchersList, ok := scrapeConfig.Spec.Params["match[]"]
	if !ok || len(matchersList) == 0 {
		return nil, nil
	}

	var parsedSelectors [][]*labels.Matcher
	for _, mStr := range matchersList {
		matchers, err := parser.ParseMetricSelector(mStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse metric selector %q: %w", mStr, err)
		}
		if len(matchers) > 0 {
			parsedSelectors = append(parsedSelectors, matchers)
		}
	}

	if len(parsedSelectors) == 0 {
		return nil, nil
	}

	var relabelConfigs []monitoringv1.RelabelConfig

	// 1. Process each selector individually to handle negation (OR disjunction semantics)
	for i, sel := range parsedSelectors {
		type posMatcher struct {
			name  string
			value string
		}

		var posMatchers []posMatcher
		for _, lm := range sel {
			if lm.Type == labels.MatchEqual || lm.Type == labels.MatchRegexp {
				var val string
				if lm.Type == labels.MatchEqual {
					val = regexp.QuoteMeta(lm.Value)
				} else {
					val = fmt.Sprintf("(?:%s)", lm.Value)
				}
				posMatchers = append(posMatchers, posMatcher{
					name:  lm.Name,
					value: val,
				})
			}
		}

		// Deterministically sort by label name, sub-sorting by regex value for stability
		slices.SortFunc(posMatchers, func(a, b posMatcher) int {
			if c := cmp.Compare(a.name, b.name); c != 0 {
				return c
			}
			return cmp.Compare(a.value, b.value)
		})

		var sourceLabels []monitoringv1.LabelName
		var posValues []string
		for _, pm := range posMatchers {
			sourceLabels = append(sourceLabels, monitoringv1.LabelName(pm.name))
			posValues = append(posValues, pm.value)
		}

		// Positive Matchers Phase (Initialize __tmp_keep_i to "keep" if metric matches positive selectors)
		tmpKeepLabel := fmt.Sprintf("__tmp_keep_%d", i)
		if len(sourceLabels) > 0 {
			relabelConfigs = append(relabelConfigs, monitoringv1.RelabelConfig{
				Action:       "replace",
				SourceLabels: sourceLabels,
				Regex:        strings.Join(posValues, ";"),
				TargetLabel:  tmpKeepLabel,
				Replacement:  ptr.To("keep"),
			})
		} else {
			relabelConfigs = append(relabelConfigs, monitoringv1.RelabelConfig{
				Action:      "replace",
				TargetLabel: tmpKeepLabel,
				Replacement: ptr.To("keep"),
			})
		}

		// Negative Matchers Phase (Clear __tmp_keep_i to "" if any negative matcher matches)
		for _, lm := range sel {
			if lm.Type == labels.MatchNotEqual || lm.Type == labels.MatchNotRegexp {
				var regexVal string
				if lm.Type == labels.MatchNotEqual {
					regexVal = regexp.QuoteMeta(lm.Value)
				} else {
					regexVal = fmt.Sprintf("(?:%s)", lm.Value)
				}

				relabelConfigs = append(relabelConfigs, monitoringv1.RelabelConfig{
					Action:       "replace",
					SourceLabels: []monitoringv1.LabelName{monitoringv1.LabelName(tmpKeepLabel), monitoringv1.LabelName(lm.Name)},
					Regex:        fmt.Sprintf("keep;%s", regexVal),
					TargetLabel:  tmpKeepLabel,
					Replacement:  ptr.To(""),
				})
			}
		}
	}

	// 2. Initialize global __tmp_keep to "drop"
	relabelConfigs = append(relabelConfigs, monitoringv1.RelabelConfig{
		Action:      "replace",
		TargetLabel: "__tmp_keep",
		Replacement: ptr.To("drop"),
	})

	// 3. Combine selector decisions: set global __tmp_keep to "keep" if any __tmp_keep_i is "keep" (OR logic)
	var combineSourceLabels []monitoringv1.LabelName
	for i := range parsedSelectors {
		combineSourceLabels = append(combineSourceLabels, monitoringv1.LabelName(fmt.Sprintf("__tmp_keep_%d", i)))
	}

	relabelConfigs = append(relabelConfigs, monitoringv1.RelabelConfig{
		Action:       "replace",
		SourceLabels: combineSourceLabels,
		Regex:        ".*keep.*",
		TargetLabel:  "__tmp_keep",
		Replacement:  ptr.To("keep"),
	})

	// 4. Keep only metrics flagged with "keep"
	relabelConfigs = append(relabelConfigs, monitoringv1.RelabelConfig{
		Action:       "keep",
		SourceLabels: []monitoringv1.LabelName{"__tmp_keep"},
		Regex:        "keep",
	})

	// 5. Cleanup all temporary labels
	relabelConfigs = append(relabelConfigs, monitoringv1.RelabelConfig{
		Action: "labeldrop",
		Regex:  "__tmp_keep.*",
	})

	// 6. Append custom metricRelabelings from scrapeConfig directly (safely deep-copied)
	for _, cfg := range scrapeConfig.Spec.MetricRelabelConfigs {
		relabelConfigs = append(relabelConfigs, *cfg.DeepCopy())
	}

	if agent == nil || len(agent.Spec.RemoteWrite) == 0 {
		baseSpec := &monitoringv1.RemoteWriteSpec{
			WriteRelabelConfigs: relabelConfigs,
		}
		return []*monitoringv1.RemoteWriteSpec{baseSpec}, nil
	}

	var specs []*monitoringv1.RemoteWriteSpec
	for _, agentRw := range agent.Spec.RemoteWrite {
		spec := &monitoringv1.RemoteWriteSpec{
			WriteRelabelConfigs: slices.Clone(relabelConfigs),
		}

		spec.URL = agentRw.URL

		if agentRw.RemoteTimeout != nil {
			spec.RemoteTimeout = ptr.To(*agentRw.RemoteTimeout)
		}
		if agentRw.BasicAuth != nil {
			spec.BasicAuth = agentRw.BasicAuth.DeepCopy()
		}
		if agentRw.Authorization != nil {
			spec.Authorization = agentRw.Authorization.DeepCopy()
		}
		if agentRw.OAuth2 != nil {
			spec.OAuth2 = agentRw.OAuth2.DeepCopy()
		}
		if agentRw.QueueConfig != nil {
			spec.QueueConfig = agentRw.QueueConfig.DeepCopy()
		}
		if agentRw.TLSConfig != nil {
			spec.TLSConfig = agentRw.TLSConfig.DeepCopy()
		}
		if agentRw.ProxyURL != nil {
			spec.ProxyURL = ptr.To(*agentRw.ProxyURL)
		}
		if agentRw.NoProxy != nil {
			spec.NoProxy = ptr.To(*agentRw.NoProxy)
		}
		if agentRw.Headers != nil {
			spec.Headers = make(map[string]string)
			maps.Copy(spec.Headers, agentRw.Headers)
		}
		if agentRw.Name != nil {
			spec.Name = ptr.To(*agentRw.Name)
		}

		specs = append(specs, spec)
	}

	return specs, nil
}
