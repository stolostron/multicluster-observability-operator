// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rule

import (
	"errors"
	"fmt"
	"os"
	"strings"

	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"sigs.k8s.io/yaml"
)

func ReadFiles(rulesPath string) ([]*prometheusv1.PrometheusRule, error) {
	paths := strings.Split(rulesPath, ",")
	ret := []*prometheusv1.PrometheusRule{}
	for _, path := range paths {
		fmt.Println("Reading prometheus rule: ", path)
		res, err := ReadFile(path)
		if err != nil {
			return nil, err
		}
		if len(res.Spec.Groups) == 0 {
			return nil, errors.New("no rule found in file")
		}
		ret = append(ret, res)
	}

	return ret, nil
}

func ReadFile(rulesPath string) (*prometheusv1.PrometheusRule, error) {
	fileData, err := os.ReadFile(rulesPath) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", rulesPath, err)
	}

	rule := &prometheusv1.PrometheusRule{}
	if err := yaml.Unmarshal(fileData, rule); err != nil {
		return nil, fmt.Errorf("failed to unmarshal file %s: %w", rulesPath, err)
	}

	return rule, nil
}

func RuleNames(rules *prometheusv1.PrometheusRule) ([]string, error) {
	ret := []string{}
	for _, rule := range rules.Spec.Groups {
		for _, rule := range rule.Rules {
			if len(rule.Alert) > 0 {
				// Is alert, skip
				continue
			}
			ret = append(ret, rule.Record)
		}
	}

	return ret, nil
}
