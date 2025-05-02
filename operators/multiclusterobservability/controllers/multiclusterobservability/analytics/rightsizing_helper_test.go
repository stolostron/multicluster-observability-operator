// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics_test

import (
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
