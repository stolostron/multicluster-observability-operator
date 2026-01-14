// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package templates

import (
	"os"
	"path"
	"testing"

	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
)

func TestGetCoreTemplates(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir %v", err)
	}
	templatesPath := path.Join(path.Dir(path.Dir(path.Dir(wd))), "manifests")
	os.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)
	defer os.Unsetenv(templatesutil.TemplatesPathEnvVar)

	_, err = GetTemplates(templatesutil.GetTemplateRenderer())
	if err != nil {
		t.Fatalf("failed to render core template %v", err)
	}
}
