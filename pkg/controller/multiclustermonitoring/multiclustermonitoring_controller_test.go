// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"os"
	"testing"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

func init() {
	os.Setenv("TEMPLATES_PATH", "../../../manifests/")
}

func TestLabelsForMultiClusterMonitoring(t *testing.T) {
	lab := labelsForMultiClusterMonitoring("test")

	value, _ := lab["monitoring.open-cluster-management.io/name"]
	if value != "test" {
		t.Errorf("value (%v) is not the expected (test)", value)
	}
}
