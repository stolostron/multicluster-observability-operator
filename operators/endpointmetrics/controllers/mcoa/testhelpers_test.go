// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"testing"

	prometheusv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func addRhobsToScheme(t *testing.T, s *runtime.Scheme) {
	t.Helper()
	gv := schema.GroupVersion{Group: "monitoring.rhobs", Version: "v1alpha1"}
	s.AddKnownTypes(gv, &prometheusv1alpha1.ScrapeConfig{}, &prometheusv1alpha1.ScrapeConfigList{})
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
