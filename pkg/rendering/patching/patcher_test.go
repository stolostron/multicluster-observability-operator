// Copyright (c) 2020 Red Hat, Inc.

package patching

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/v3/k8sdeps/kunstruct"
	"sigs.k8s.io/kustomize/v3/pkg/resource"
	"sigs.k8s.io/yaml"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-observability-operator/pkg/apis/monitoring/v1alpha1"
)

var apiserver = `
kind: Deployment
apiVersion: apps/v1
metadata:
  name: mcm-apiserver
  labels:
    app: "mcm-apiserver"
spec:
  template:
    spec:
      volumes:
        - name: apiserver-cert
          secret:
            secretName: "test"
      containers:
      - name: mcm-apiserver
        image: "mcm-api"
        env:
          - name: MYHUBNAME
            value: test
        volumeMounts: []
        args:
          - "/mcm-apiserver"
          - "--enable-admission-plugins=HCMUserIdentity,KlusterletCA,NamespaceLifecycle"
`

var factory = resource.NewFactory(kunstruct.NewKunstructuredFactoryImpl())

func TestApplyGlobalPatches(t *testing.T) {
	json, err := yaml.YAMLToJSON([]byte(apiserver))
	if err != nil {
		t.Fatalf("failed to apply global patches %v", err)
	}
	var u unstructured.Unstructured
	u.UnmarshalJSON(json)
	apiserver := factory.FromMap(u.Object)

	mchcr := &monitoringv1alpha1.MultiClusterMonitoring{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterMonitoring"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test"},
		Spec: monitoringv1alpha1.MultiClusterMonitoringSpec{
			ImageRepository: "quay.io/open-cluster-management",
			ImagePullPolicy: "Always",
			ImagePullSecret: "test",
		},
	}

	err = ApplyGlobalPatches(apiserver, mchcr)
	if err != nil {
		t.Fatalf("failed to apply global patches %v", err)
	}
}
