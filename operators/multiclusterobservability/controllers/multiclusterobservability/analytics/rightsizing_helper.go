// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"github.com/cloudflare/cfssl/log"
	"gopkg.in/yaml.v2"
)

// FormatYAML converts a Go data structure to a YAML-formatted string
func FormatYAML(data interface{}) string {
	defer func() {
		if r := recover(); r != nil {
			log.Error(nil, "Panic recovered during YAML marshaling: %v", r)
		}
	}()

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		log.Error(err, "Error marshaling data to YAML: %v")
		return ""
	}
	return string(yamlData)
}
