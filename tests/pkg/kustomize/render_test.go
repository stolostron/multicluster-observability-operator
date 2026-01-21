// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package kustomize

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestRender(t *testing.T) {
	// Test
	buf, err := Render(Options{
		KustomizationPath: "tests",
	})
	require.NoError(t, err, "Render()")
	rendered := rendered(t, buf)
	names := containedNames(rendered)
	assert.Equal(t, []string{"thanos-ruler-custom-rules"}, names, "rendered names")

	labels, _ := GetLabels(buf)
	for labelName := range labels.(map[string]any) {
		assert.Equal(t, "alertname", labelName, "metadata label")
	}

	str := string(buf)
	pkgLabelPos := strings.Index(str, "alertname: NodeOutOfMemory\n")
	assert.True(t, pkgLabelPos > 0, "alertname: NodeOutOfMemory label should be contained")
}

func containedNames(rendered []map[string]any) (names []string) {
	for _, o := range rendered {
		m, ok := o["metadata"].(map[string]any)
		if ok {
			names = append(names, m["name"].(string))
		}
	}
	return
}

func rendered(t *testing.T, rendered []byte) (r []map[string]any) {
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(rendered), 4096)
	for {
		o := map[string]any{}
		if err := dec.Decode(&o); err != nil {
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		if len(o) > 0 {
			r = append(r, o)
		}
	}
	return
}
