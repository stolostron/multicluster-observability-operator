// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

func TestGenerateDashboardMetricCM(t *testing.T) {
	mco := &monitoringv1alpha1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: dashboardMetricsConfigMapNS, Name: "name"},
		Spec:       monitoringv1alpha1.MultiClusterMonitoringSpec{},
	}

	s := scheme.Scheme
	monitoringv1alpha1.SchemeBuilder.AddToScheme(s)
	objs := []runtime.Object{mco}
	cl := fake.NewFakeClient(objs...)

	caseList := []struct {
		name     string
		expected error
	}{
		{
			name:     "create cm",
			expected: nil,
		},

		{
			name:     "create cm when cm is existing",
			expected: nil,
		},
	}

	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			_, err := GenerateDashboardMetricCM(cl, s, mco)
			if err != c.expected {
				t.Errorf("case (%v) err (%v) is not the expected (%v)", c.name, err, c.expected)
			}
		})
	}
}

func TestGetDashboardMetrics(t *testing.T) {
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
				Data: map[string]string{dashboardMetricsConfigMapKey: `
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
				Data: map[string]string{dashboardMetricsConfigMapKey: ""},
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
				Data: map[string]string{dashboardMetricsConfigMapKey: `
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
				Data: map[string]string{dashboardMetricsConfigMapKey: ""},
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
