// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"testing"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	prometheusv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

func addRhobsToScheme(t *testing.T, s *runtime.Scheme) {
	t.Helper()
	gv := schema.GroupVersion{Group: "monitoring.rhobs", Version: "v1alpha1"}
	s.AddKnownTypes(gv,
		&prometheusv1alpha1.ScrapeConfig{}, &prometheusv1alpha1.ScrapeConfigList{},
		&prometheusv1alpha1.PrometheusAgent{}, &prometheusv1alpha1.PrometheusAgentList{},
	)
	metav1.AddToGroupVersion(s, gv)
}

func newRawScrapeConfig(name, namespace, component string) *prometheusv1alpha1.ScrapeConfig {
	return &prometheusv1alpha1.ScrapeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				labelKeyComponent: component,
			},
		},
		Spec: prometheusv1alpha1.ScrapeConfigSpec{
			Params: map[string][]string{
				"match[]": {
					`{__name__="up"}`,
				},
			},
		},
	}
}

func newTestPrometheusAgent(name, namespace, component, rwURL, caSecretName, certSecretName string) *prometheusv1alpha1.PrometheusAgent {
	agent := &prometheusv1alpha1.PrometheusAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				labelKeyComponent: component,
			},
		},
		Spec: prometheusv1alpha1.PrometheusAgentSpec{
			CommonPrometheusFields: monitoringv1.CommonPrometheusFields{
				RemoteWrite: []monitoringv1.RemoteWriteSpec{
					{
						Name: ptr.To("acm-observability"),
						URL:  rwURL,
					},
				},
			},
		},
	}

	if caSecretName != "" || certSecretName != "" {
		tlsConfig := &monitoringv1.TLSConfig{
			SafeTLSConfig: monitoringv1.SafeTLSConfig{},
		}
		if caSecretName != "" {
			tlsConfig.CA = monitoringv1.SecretOrConfigMap{
				Secret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: caSecretName,
					},
					Key: "ca.crt",
				},
			}
		}
		if certSecretName != "" {
			tlsConfig.Cert = monitoringv1.SecretOrConfigMap{
				Secret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: certSecretName,
					},
					Key: "tls.crt",
				},
			}
			tlsConfig.KeySecret = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: certSecretName,
				},
				Key: "tls.key",
			}
		}
		agent.Spec.RemoteWrite[0].TLSConfig = tlsConfig
	}

	return agent
}
