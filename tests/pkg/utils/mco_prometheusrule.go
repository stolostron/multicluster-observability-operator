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
	"k8s.io/apimachinery/pkg/util/intstr"
)

func CreatePrometheusRule(opt TestOptions, name, namespace, componentLabel, metricName string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	rule := &prometheusv1.PrometheusRule{
		TypeMeta: metav1.TypeMeta{
			APIVersion: prometheusv1.SchemeGroupVersion.String(),
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

	ruleMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(rule)
	if err != nil {
		return err
	}

	ruleUnstructured := &unstructured.Unstructured{Object: ruleMap}

	_, err = clientDynamic.Resource(NewPrometheusRuleGVR()).Namespace(namespace).Create(context.TODO(), ruleUnstructured, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			existing, err := clientDynamic.Resource(NewPrometheusRuleGVR()).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			ruleUnstructured.SetResourceVersion(existing.GetResourceVersion())
			_, err = clientDynamic.Resource(NewPrometheusRuleGVR()).Namespace(namespace).Update(context.TODO(), ruleUnstructured, metav1.UpdateOptions{})
			return err
		}
	}
	return err
}

func DeletePrometheusRule(opt TestOptions, name, namespace string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	return clientDynamic.Resource(NewPrometheusRuleGVR()).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}
