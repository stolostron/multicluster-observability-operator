// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package collector

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEvluateMatchExpression(t *testing.T) {
	caseList := []struct {
		name           string
		expr           metav1.LabelSelectorRequirement
		clusterType    string
		expectedResult bool
	}{
		{
			name: "unsupported key",
			expr: metav1.LabelSelectorRequirement{
				Key:      "test_key",
				Operator: "In",
				Values:   []string{"test_value"},
			},
			expectedResult: false,
		},
		{
			name: "unsupported expr operator",
			expr: metav1.LabelSelectorRequirement{
				Key:      "clusterType",
				Operator: "test_op",
				Values:   []string{"test_value"},
			},
			expectedResult: false,
		},
		{
			name: "filter non-SNO rule in SNO",
			expr: metav1.LabelSelectorRequirement{
				Key:      "clusterType",
				Operator: "NotIn",
				Values:   []string{"SNO"},
			},
			clusterType:    "SNO",
			expectedResult: false,
		},
		{
			name: "filter SNO rule in non-SNO",
			expr: metav1.LabelSelectorRequirement{
				Key:      "clusterType",
				Operator: "In",
				Values:   []string{"SNO"},
			},
			clusterType:    "",
			expectedResult: false,
		},
		{
			name: "select non-SNO rule in non-SNO",
			expr: metav1.LabelSelectorRequirement{
				Key:      "clusterType",
				Operator: "NotIn",
				Values:   []string{"SNO"},
			},
			clusterType:    "",
			expectedResult: true,
		},
		{
			name: "select SNO rule in SNO",
			expr: metav1.LabelSelectorRequirement{
				Key:      "clusterType",
				Operator: "In",
				Values:   []string{"SNO"},
			},
			clusterType:    "SNO",
			expectedResult: true,
		},
	}

	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			// params := append([]interface{}{"id"}, c.clusterType)
			r := evaluateMatchExpression(c.expr, "clusterType", c.clusterType)
			if r != c.expectedResult {
				t.Fatalf("Wrong result for test %s, expected %v, got %v", c.name, c.expectedResult, r)
			}
		})
	}
}
