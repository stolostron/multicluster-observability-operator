// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"context"
	"testing"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestApplyPrometheusRule_CreateThenUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, monitoringv1.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	pr := monitoringv1.PrometheusRule{
		TypeMeta: metav1.TypeMeta{APIVersion: "monitoring.coreos.com/v1", Kind: "PrometheusRule"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rule",
			Namespace: "openshift-monitoring",
			Labels:    map[string]string{"a": "b"},
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{{Name: "g1"}},
		},
	}

	require.NoError(t, ApplyPrometheusRule(ctx, c, pr))

	pr2 := pr.DeepCopy()
	pr2.Labels = map[string]string{"a": "c"}
	pr2.Spec.Groups = []monitoringv1.RuleGroup{{Name: "g2"}}
	require.NoError(t, ApplyPrometheusRule(ctx, c, *pr2))

	got := &monitoringv1.PrometheusRule{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: "openshift-monitoring", Name: "test-rule"}, got))
	assert.Equal(t, "c", got.Labels["a"])
	assert.Equal(t, "g2", got.Spec.Groups[0].Name)
}
