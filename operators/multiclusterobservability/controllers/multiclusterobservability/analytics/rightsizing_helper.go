// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"encoding/json"
	"fmt"

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

// Function to add "apiVersion" and "kind" to a Kubernetes object
func AddAPIVersionAndKind(obj interface{}, apiVersion, kind string) ([]byte, error) {

	// Step 1: Marshal object to JSON
	objJSON, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("error marshaling object: %w", err)
	}

	// Step 2: Convert JSON into a map to modify fields
	var objMap map[string]interface{}
	if err := json.Unmarshal(objJSON, &objMap); err != nil {
		return nil, fmt.Errorf("error marshaling JSON: %w", err)
	}

	// Step 3: Inject "apiVersion" and "kind"
	objMap["apiVersion"] = apiVersion
	objMap["kind"] = kind

	// Step 4: Convert back to JSON
	finalJSON, err := json.Marshal(objMap)
	if err != nil {
		return nil, fmt.Errorf("error re-marshaling JSON: %w", err)
	}

	return finalJSON, nil
}
