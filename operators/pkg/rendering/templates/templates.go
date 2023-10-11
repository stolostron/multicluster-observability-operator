// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package templates

import (
	"log"
	"os"
	"sync"

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

const (
	TemplatesPathEnvVar = "TEMPLATES_PATH"
)

var (
	loadTemplateRendererOnce sync.Once
	templateRenderer         *TemplateRenderer
	templatesPath            = "/usr/local/manifests"
)

func GetTemplateRenderer() *TemplateRenderer {
	loadTemplateRendererOnce.Do(func() {
		templatesPathInEnv, found := os.LookupEnv(TemplatesPathEnvVar)
		if found {
			templatesPath = templatesPathInEnv
		}
		templateRenderer = &TemplateRenderer{
			templatesPath: templatesPath,
			templates:     map[string]resmap.ResMap{},
		}
	})
	return templateRenderer
}

type TemplateRenderer struct {
	templatesPath string
	templates     map[string]resmap.ResMap
}

func NewTemplateRenderer(path string) *TemplateRenderer {
	return &TemplateRenderer{
		templatesPath: path,
		templates:     map[string]resmap.ResMap{},
	}
}

func (r *TemplateRenderer) GetTemplatesPath() string {
	return r.templatesPath
}

func (r *TemplateRenderer) AddTemplateFromPath(kustomizationPath string, resourceList *[]*resource.Resource) error {
	var err error
	resMap, ok := r.templates[kustomizationPath]
	if !ok {
		resMap, err = r.render(kustomizationPath)
		if err != nil {
			log.Printf("Cannot find this path %v, %v", kustomizationPath, err)
			return nil
		}
		r.templates[kustomizationPath] = resMap
	}
	*resourceList = append(*resourceList, resMap.Resources()...)
	return nil
}

func (r *TemplateRenderer) render(kustomizationPath string) (resmap.ResMap, error) {
	fs := filesys.MakeFsOnDisk()
	opts := krusty.MakeDefaultOptions()
	k := krusty.MakeKustomizer(opts)
	m, err := k.Run(fs, kustomizationPath)
	if err != nil {
		return nil, err
	}
	return m, nil
}
