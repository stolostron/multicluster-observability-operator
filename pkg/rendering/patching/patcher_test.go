// Copyright (c) 2020 Red Hat, Inc.

package patching

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/v3/k8sdeps/kunstruct"
	"sigs.k8s.io/kustomize/v3/pkg/resource"
	"sigs.k8s.io/yaml"

	mcov1beta1 "github.com/stolostron/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
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

	mchcr := &mcov1beta1.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "test",
			Annotations: map[string]string{"mco-imageRepository": "quay.io/stolostron"},
		},
		Spec: mcov1beta1.MultiClusterObservabilitySpec{
			ImagePullPolicy: "Always",
			ImagePullSecret: "test",
		},
	}

	err = ApplyGlobalPatches(apiserver, mchcr)
	if err != nil {
		t.Fatalf("failed to apply global patches %v", err)
	}
}
