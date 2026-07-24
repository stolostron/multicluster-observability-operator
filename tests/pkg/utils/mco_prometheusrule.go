// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"

	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func createPrometheusRuleGeneric(ctx context.Context, opt TestOptions, name, namespace, componentLabel, metricName, targetNamespace string, gvr schema.GroupVersionResource, apiVersion string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	rule := &prometheusv1.PrometheusRule{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiVersion,
			Kind:       prometheusv1.PrometheusRuleKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": componentLabel,
			},
		},
		Spec: prometheusv1.PrometheusRuleSpec{
			Groups: []prometheusv1.RuleGroup{
				{
					Name: "test-rules",
					Rules: []prometheusv1.Rule{
						{
							Record: metricName,
							Expr:   intstr.FromString("vector(1)"),
						},
					},
				},
			},
		},
	}

	if targetNamespace != "" {
		rule.Annotations = map[string]string{
			"observability.open-cluster-management.io/target-namespace": targetNamespace,
		}
		rule.Labels["openshift.io/prometheus-rule-evaluation-scope"] = "leaf-prometheus"
	}

	ruleMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(rule)
	if err != nil {
		return err
	}

	ruleUnstructured := &unstructured.Unstructured{Object: ruleMap}

	_, err = clientDynamic.Resource(gvr).Namespace(namespace).Create(ctx, ruleUnstructured, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			existing, err := clientDynamic.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			existing.Object["spec"] = ruleUnstructured.Object["spec"]
			_, err = clientDynamic.Resource(gvr).Namespace(namespace).Update(ctx, existing, metav1.UpdateOptions{})
			return err
		}
	}
	return err
}

func CreatePrometheusRule(ctx context.Context, opt TestOptions, name, namespace, componentLabel, metricName, targetNamespace string) error {
	return createPrometheusRuleGeneric(ctx, opt, name, namespace, componentLabel, metricName, targetNamespace, NewPrometheusRuleGVR(), prometheusv1.SchemeGroupVersion.String())
}

func DeletePrometheusRule(ctx context.Context, opt TestOptions, name, namespace string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	return clientDynamic.Resource(NewPrometheusRuleGVR()).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func CreateMCOAPrometheusRule(ctx context.Context, opt TestOptions, name, namespace, componentLabel, metricName, targetNamespace string) error {
	return createPrometheusRuleGeneric(ctx, opt, name, namespace, componentLabel, metricName, targetNamespace, NewMCOAPrometheusRuleGVR(), "monitoring.rhobs/v1")
}

func DeleteMCOAPrometheusRule(ctx context.Context, opt TestOptions, name, namespace string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	return clientDynamic.Resource(NewMCOAPrometheusRuleGVR()).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}
