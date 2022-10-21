// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"reflect"
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
			operatorconfig.MetricsOcp311ConfigMapKey: `
names:
  - ocp311_a
  - ocp311_b
`},
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
`},
	}
}

func TestMergeAllowList(t *testing.T) {
	c := fake.NewFakeClient(getAllowlistCM(), geCustomAllowlistCM())
	allowlist, ocp3Allowlist, uwlAllowlist, err := GetAllowList(c, operatorconfig.AllowlistConfigMapName,
		config.GetDefaultNamespace())
	if err != nil {
		t.Errorf("Failed to get allowlist: (%v)", err)
	}
	customAllowlist, _, customUwlAllowlist, err := GetAllowList(c, operatorconfig.AllowlistCustomConfigMapName,
		config.GetDefaultNamespace())
	if err != nil {
		t.Errorf("Failed to get allowlist: (%v)", err)
	}
	list, ocp3List, uwlList := MergeAllowlist(allowlist, customAllowlist, ocp3Allowlist,
		uwlAllowlist, customUwlAllowlist)
	if !Contains(list.NameList, "custom_a") {
		t.Error("metrics custom_a not merged into allowlist")
	}
	if !Contains(ocp3List.NameList, "custom_a") {
		t.Error("metrics custom_a not merged into allowlist")
	}
	if !Contains(uwlList.NameList, "custom_uwl_a") {
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
