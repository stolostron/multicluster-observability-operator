// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics_test

import (
	"encoding/json"
	"testing"

	analyticsctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics"
	"github.com/stretchr/testify/assert"
)

func TestFormatYAML_ValidData(t *testing.T) {
	input := map[string]string{
		"key": "value",
	}
	output := analyticsctrl.FormatYAML(input)
	assert.Contains(t, output, "key: value")
}

func TestFormatYAML_InvalidData(t *testing.T) {
	// YAML marshaling will fail on channels or functions
	input := make(chan int)
	output := analyticsctrl.FormatYAML(input)
	assert.Equal(t, "", output) // Should return empty string if marshal fails
}

func TestAddAPIVersionAndKind_Success(t *testing.T) {
	obj := map[string]interface{}{
		"name": "test",
	}
	version := "v1"
	kind := "TestKind"

	result, err := analyticsctrl.AddAPIVersionAndKind(obj, version, kind)
	assert.NoError(t, err)

	var out map[string]interface{}
	err = json.Unmarshal(result, &out)
	assert.NoError(t, err)
	assert.Equal(t, version, out["apiVersion"])
	assert.Equal(t, kind, out["kind"])
	assert.Equal(t, "test", out["name"])
}

func TestAddAPIVersionAndKind_InvalidInput(t *testing.T) {
	// Non-marshallable struct (e.g., with a channel field)
	type badType struct {
		Ch chan int
	}
	obj := badType{Ch: make(chan int)}

	_, err := analyticsctrl.AddAPIVersionAndKind(obj, "v1", "BadKind")
	assert.Error(t, err)
}
