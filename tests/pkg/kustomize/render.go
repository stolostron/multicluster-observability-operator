// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package kustomize

import (
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/yaml"
)

// Options ...
type Options struct {
	KustomizationPath string
	OutputPath        string
}

// Render is used to render the kustomization
func Render(o Options) ([]byte, error) {
	fSys := filesys.MakeFsOnDisk()
	k := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	m, err := k.Run(fSys, o.KustomizationPath)
	if err != nil {
		return nil, err
	}
	return m.AsYaml()
}

// GetLabels return labels
func GetLabels(yamlB []byte) (interface{}, error) {
	data := map[string]interface{}{}
	err := yaml.Unmarshal(yamlB, &data)
	return data["metadata"].(map[string]interface{})["labels"], err
}
