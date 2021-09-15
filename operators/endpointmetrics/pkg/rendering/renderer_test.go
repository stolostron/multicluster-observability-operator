// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"os"
	"path"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/config"
	rendererutil "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering"
	templatesutil "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering/templates"
)

func getAllowlistCM() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.AllowlistConfigMapName,
			Namespace: namespace,
		},
		Data: map[string]string{
			metricsConfigMapKey: `
names:
  - apiserver_watch_events_sizes_bucket
`},
	}
}

func TestRender(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir %v", err)
	}
	templatesPath := path.Join(path.Dir(path.Dir(wd)), "manifests")
	os.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)
	defer os.Unsetenv(templatesutil.TemplatesPathEnvVar)

	renderer := rendererutil.NewRenderer()
	hubInfo := &operatorconfig.HubInfo{
		ClusterName:              "foo",
		ObservatoriumAPIEndpoint: "testing.com",
		AlertmanagerEndpoint:     "testing.com",
		AlertmanagerRouterCA:     "testing",
	}

	c := fake.NewFakeClient([]runtime.Object{getAllowlistCM()}...)

	objs, err := Render(renderer, c, hubInfo)
	if err != nil {
		t.Fatalf("failed to render endpoint templates: %v", err)
	}

	printObjs(t, objs)
}

func printObjs(t *testing.T, objs []*unstructured.Unstructured) {
	for _, obj := range objs {
		t.Log(obj)
	}
}
