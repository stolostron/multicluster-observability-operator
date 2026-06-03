// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"

	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func getAllowlistCM(ns string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.AllowlistConfigMapName,
			Namespace: ns,
		},
		Data: map[string]string{
			metricsConfigMapKey: `
names:
  - apiserver_watch_events_sizes_bucket
`,
		},
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
	c := fake.NewClientBuilder().WithRuntimeObjects([]runtime.Object{getAllowlistCM("test-ns")}...).Build()

	objs, err := Render(context.Background(), renderer, c, hubInfo, "test-ns")
	if err != nil {
		t.Fatalf("failed to render endpoint templates: %v", err)
	}

	assert.Greater(t, len(objs), 2)
}

func TestRenderAlertmanagerConfig(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir %v", err)
	}
	templatesPath := path.Join(path.Dir(path.Dir(wd)), "manifests")
	os.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)
	defer os.Unsetenv(templatesutil.TemplatesPathEnvVar)

	const hubClusterID = "test"
	hubInfo := &operatorconfig.HubInfo{
		ClusterName:              "foo",
		ObservatoriumAPIEndpoint: "testing.com",
		AlertmanagerEndpoint:     "testing.com",
		HubClusterID:             hubClusterID,
	}
	c := fake.NewClientBuilder().WithRuntimeObjects([]runtime.Object{getAllowlistCM("test-ns")}...).Build()

	objs, err := Render(context.Background(), rendererutil.NewRenderer(), c, hubInfo, "test-ns")
	if err != nil {
		t.Fatalf("failed to render endpoint templates: %v", err)
	}

	var amConfig string
	for _, obj := range objs {
		if obj.GetKind() == "Secret" && obj.GetName() == "prometheus-alertmanager" {
			amConfig = obj.Object["stringData"].(map[string]any)["alertmanager.yaml"].(string)
			break
		}
	}
	if amConfig == "" {
		t.Fatal("prometheus-alertmanager secret not found in rendered objects")
	}

	// check for mtls and hub id
	want := []string{
		"/etc/prometheus/secrets/observability-alertmanager-accessor-" + hubClusterID + "/token",
		"/etc/prometheus/secrets/obs-alertmanager-mtls-ca-" + hubClusterID + "/ca.crt",
		"/etc/prometheus/secrets/obs-alertmanager-mtls-cert-" + hubClusterID + "/tls.crt",
		"/etc/prometheus/secrets/obs-alertmanager-mtls-cert-" + hubClusterID + "/tls.key",
	}
	for _, path := range want {
		if !strings.Contains(amConfig, path) {
			t.Fatalf("alertmanager.yaml missing path %q:\n%s", path, amConfig)
		}
	}
}
