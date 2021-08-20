// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package templates

import (
	"os"
	"path"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	templatesutil "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering/templates"
)

func TestGetCoreTemplates(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir %v", err)
	}
	templatesPath := path.Join(path.Dir(path.Dir(path.Dir(wd))), "manifests")
	os.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)
	defer os.Unsetenv(templatesutil.TemplatesPathEnvVar)

	mchcr := &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			ImagePullPolicy: "Always",
			ImagePullSecret: "test",
			StorageConfig: &mcov1beta2.StorageConfig{
				StorageClass: "gp2",
			},
		},
	}
	_, err = GetTemplates(templatesutil.GetTemplateRenderer(), mchcr)

	if err != nil {
		t.Fatalf("failed to render core template %v", err)
	}
}
