// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"os"
	"path"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	operatorconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/config"
	rendererutil "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering"
	templatesutil "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering/templates"
)

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
	objs, err := Render(renderer, nil, hubInfo)
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
