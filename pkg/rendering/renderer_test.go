// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"os"
	"path"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	mcoshared "github.com/stolostron/multicluster-observability-operator/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/pkg/rendering/templates"
)

func TestRender(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir %v", err)
	}
	templatesPath := path.Join(path.Dir(path.Dir(wd)), "manifests")
	os.Setenv(templates.TemplatesPathEnvVar, templatesPath)
	defer os.Unsetenv(templates.TemplatesPathEnvVar)

	mchcr := &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			ImagePullPolicy: "Always",
			ImagePullSecret: "test",
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
				StorageClass:            "gp2",
				AlertmanagerStorageSize: "1Gi",
				CompactStorageSize:      "1Gi",
				RuleStorageSize:         "1Gi",
				ReceiveStorageSize:      "1Gi",
				StoreStorageSize:        "1Gi",
			},
		},
	}

	renderer := NewRenderer(mchcr)
	objs, err := renderer.Render(nil)
	if err != nil {
		t.Fatalf("failed to render MultiClusterObservability: %v", err)
	}

	printObjs(t, objs)
}

func printObjs(t *testing.T, objs []*unstructured.Unstructured) {
	for _, obj := range objs {
		t.Log(obj)
	}
}
