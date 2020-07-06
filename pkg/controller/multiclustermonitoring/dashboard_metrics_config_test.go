// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDashboardMetrics(t *testing.T) {
	internalDefaultMetricsLen := 58
	caseList := []struct {
		name     string
		expected int
		cm       corev1.ConfigMap
	}{
		{
			name:     "valid configmap",
			expected: internalDefaultMetricsLen + 4,
			cm: corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardMetricsConfigMapName,
					Namespace: dashboardMetricsConfigMapNS,
				},
				Data: map[string]string{"metrics.yaml": `
defalutMetrics:
  - a
  - b
additionalMetrics:
  - c
  - d
`},
			},
		},

		{
			name:     "invalid configmap",
			expected: internalDefaultMetricsLen,
			cm: corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "ns",
				},
				Data: map[string]string{"metrics.yaml": ""},
			},
		},

		{
			name:     "invalid yaml format",
			expected: internalDefaultMetricsLen,
			cm: corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardMetricsConfigMapName,
					Namespace: dashboardMetricsConfigMapNS,
				},
				Data: map[string]string{"metrics.yaml": `
defalutMetrics
  - a
  - b
additionalMetrics
  - c
  - d
`},
			},
		},

		{
			name:     "empty data",
			expected: internalDefaultMetricsLen,
			cm: corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardMetricsConfigMapName,
					Namespace: dashboardMetricsConfigMapNS,
				},
				Data: map[string]string{"metrics.yaml": ""},
			},
		},
	}

	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			objs := []runtime.Object{}
			cl := fake.NewFakeClient(objs...)
			err := cl.Create(context.TODO(), &c.cm)
			if err != nil {
				t.Errorf("failed to create configmap (%v), err: (%v)", c.cm.Name, err)
			}
			metrics := getDashboardMetrics(cl)
			if len(metrics) != c.expected {
				t.Errorf("case (%v) len(metrics) (%v) is not the expected (%v)", c.name, len(metrics), c.expected)
			}
			err = cl.Delete(context.TODO(), &c.cm)
			if err != nil {
				t.Errorf("failed to delete configmap (%v), err: (%v)", c.cm.Name, err)
			}

		})
	}
}
