// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"reflect"
	"slices"
	"testing"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func getAllowlistCM() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.AllowlistConfigMapName,
			Namespace: config.GetDefaultNamespace(),
		},
		Data: map[string]string{
			operatorconfig.MetricsConfigMapKey: `
names:
  - a
  - b
matches:
  - __name__="c"
recording_rules:
  - record: f
    expr: g
collect_rules:
  - name: h
    selector:
      matchExpressions:
        - key: clusterType
          operator: NotIn
          values: ["SNO"]
    rules:
      - collect: j
        expr: k
        for: 1m
        names:
          - c
        matches:
          - __name__="a"
`,
			operatorconfig.UwlMetricsConfigMapKey: `
names:
  - uwl_a
  - uwl_b
`,
		},
	}
}

func geCustomAllowlistCM() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.AllowlistCustomConfigMapName,
			Namespace: config.GetDefaultNamespace(),
		},
		Data: map[string]string{
			operatorconfig.MetricsConfigMapKey: `
names:
  - custom_a
  - custom_b
matches:
  - __name__="custom_c"
recording_rules:
  - record: custom_f
    expr: custom_g
collect_rules:
  - name: -h
  - name: custom_h
    selector:
      matchExpressions:
        - key: clusterType
          operator: NotIn
          values: ["SNO"]
    rules:
      - collect: j
        expr: k
        for: 1m
        names:
          - c
        matches:
          - __name__="a"
`,
			operatorconfig.UwlMetricsConfigMapKey: `
names:
  - custom_uwl_a
  - custom_uwl_b
`,
		},
	}
}

func TestMergeAllowList(t *testing.T) {
	c := fake.NewClientBuilder().WithRuntimeObjects(getAllowlistCM(), geCustomAllowlistCM()).Build()
	allowlist, uwlAllowlist, err := GetAllowList(t.Context(), c, operatorconfig.AllowlistConfigMapName,
		config.GetDefaultNamespace())
	if err != nil {
		t.Errorf("Failed to get allowlist: (%v)", err)
	}
	customAllowlist, customUwlAllowlist, err := GetAllowList(t.Context(), c, operatorconfig.AllowlistCustomConfigMapName,
		config.GetDefaultNamespace())
	if err != nil {
		t.Errorf("Failed to get allowlist: (%v)", err)
	}
	list, uwlList := MergeAllowlist(allowlist, customAllowlist,
		uwlAllowlist, customUwlAllowlist)
	if !slices.Contains(list.NameList, "custom_a") {
		t.Error("metrics custom_a not merged into allowlist")
	}
	if !slices.Contains(uwlList.NameList, "custom_uwl_a") {
		t.Error("metrics custom_uwl_a not merged into uwl allowlist")
	}
}

func TestMergeMetrics(t *testing.T) {
	testCaseList := []struct {
		name             string
		defaultAllowlist []string
		customAllowlist  []string
		want             []string
	}{
		{
			name:             "no deleted metrics",
			defaultAllowlist: []string{"a", "b"},
			customAllowlist:  []string{"c"},
			want:             []string{"a", "b", "c"},
		},

		{
			name:             "no default metrics",
			defaultAllowlist: []string{},
			customAllowlist:  []string{"a"},
			want:             []string{"a"},
		},

		{
			name:             "no metrics",
			defaultAllowlist: []string{},
			customAllowlist:  []string{},
			want:             []string{},
		},

		{
			name:             "have deleted metrics",
			defaultAllowlist: []string{"a", "b"},
			customAllowlist:  []string{"c", "-b"},
			want:             []string{"a", "c"},
		},

		{
			name:             "have deleted matches",
			defaultAllowlist: []string{"__name__=\"a\",job=\"a\"", "__name__=\"b\",job=\"b\""},
			customAllowlist:  []string{"-__name__=\"b\",job=\"b\"", "__name__=\"c\",job=\"c\""},
			want:             []string{"__name__=\"a\",job=\"a\"", "__name__=\"c\",job=\"c\""},
		},

		{
			name:             "deleted metrics is no exist",
			defaultAllowlist: []string{"a", "b"},
			customAllowlist:  []string{"c", "-d"},
			want:             []string{"a", "b", "c"},
		},

		{
			name:             "deleted all metrics",
			defaultAllowlist: []string{"a", "b"},
			customAllowlist:  []string{"-a", "-b"},
			want:             []string{},
		},

		{
			name:             "delete custorm metrics",
			defaultAllowlist: []string{"a", "b"},
			customAllowlist:  []string{"a", "-a"},
			want:             []string{"b"},
		},

		{
			name:             "have repeated default metrics",
			defaultAllowlist: []string{"a", "a"},
			customAllowlist:  []string{"a", "-b"},
			want:             []string{"a"},
		},

		{
			name:             "have repeated custom metrics",
			defaultAllowlist: []string{"a"},
			customAllowlist:  []string{"b", "b", "-a"},
			want:             []string{"b"},
		},
	}

	for _, c := range testCaseList {
		got := mergeMetrics(c.defaultAllowlist, c.customAllowlist)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: mergeMetrics() = %v, want %v", c.name, got, c.want)
		}
	}
}
