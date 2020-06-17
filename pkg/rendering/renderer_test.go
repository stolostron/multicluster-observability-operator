// Copyright (c) 2020 Red Hat, Inc.

package rendering

import (
	"os"
	"path"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/rendering/templates"
)

func TestRender(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir %v", err)
	}
	templatesPath := path.Join(path.Dir(path.Dir(wd)), "manifests")
	os.Setenv(templates.TemplatesPathEnvVar, templatesPath)
	defer os.Unsetenv(templates.TemplatesPathEnvVar)

	mchcr := &monitoringv1alpha1.MultiClusterMonitoring{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterMonitoring"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec: monitoringv1alpha1.MultiClusterMonitoringSpec{
			Version:         "latest",
			ImageRepository: "quay.io/open-cluster-management",
			ImagePullPolicy: "Always",
			ImagePullSecret: "test",
			StorageClass:    "gp2",

			ObjectStorageConfigSpec: &monitoringv1alpha1.ObjectStorageConfigSpec{
				Type: "minio",
				Config: monitoringv1alpha1.ObjectStorageConfig{
					Bucket:    "Bucket",
					Endpoint:  "Endpoint",
					Insecure:  true,
					AccessKey: "AccessKey",
					SecretKey: "SecretKey",
					Storage:   "Storage",
				},
			},
			Grafana: &monitoringv1alpha1.GrafanaSpec{
				Hostport: 3001,
				Replicas: 1,
			},
		},
	}

	renderer := NewRenderer(mchcr)
	objs, err := renderer.Render(nil)
	if err != nil {
		t.Fatalf("failed to render MultiClusterMonitoring %v", err)
	}

	printObjs(t, objs)
}

func printObjs(t *testing.T, objs []*unstructured.Unstructured) {
	for _, obj := range objs {
		t.Log(obj)
	}
}
