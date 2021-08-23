// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package templates

import (
	"log"
	"os"
	"sync"

	"sigs.k8s.io/kustomize/v3/k8sdeps/kunstruct"
	"sigs.k8s.io/kustomize/v3/k8sdeps/transformer"
	"sigs.k8s.io/kustomize/v3/k8sdeps/validator"
	"sigs.k8s.io/kustomize/v3/pkg/fs"
	"sigs.k8s.io/kustomize/v3/pkg/loader"
	"sigs.k8s.io/kustomize/v3/pkg/plugins"
	"sigs.k8s.io/kustomize/v3/pkg/resmap"
	"sigs.k8s.io/kustomize/v3/pkg/resource"
	"sigs.k8s.io/kustomize/v3/pkg/target"
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
	ldr, err := loader.NewLoader(
		loader.RestrictionRootOnly,
		validator.NewKustValidator(),
		kustomizationPath,
		fs.MakeFsOnDisk(),
	)

	if err != nil {
		return nil, err
	}
	defer func() {
		if err := ldr.Cleanup(); err != nil {
			log.Printf("failed to clean up loader, %v", err)
		}
	}()
	pf := transformer.NewFactoryImpl()
	rf := resmap.NewFactory(resource.NewFactory(kunstruct.NewKunstructuredFactoryImpl()), pf)
	pl := plugins.NewLoader(plugins.DefaultPluginConfig(), rf)
	kt, err := target.NewKustTarget(ldr, rf, pf, pl)
	if err != nil {
		return nil, err
	}
	return kt.MakeCustomizedResMap()
}
