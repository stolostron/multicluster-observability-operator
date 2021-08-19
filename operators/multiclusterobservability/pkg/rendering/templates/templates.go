// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package templates

import (
	"log"
	"os"
	"path"
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

const TemplatesPathEnvVar = "TEMPLATES_PATH"

var loadTemplateRendererOnce sync.Once
var templateRenderer *TemplateRenderer

// *Templates contains all kustomize resources
var genericTemplates, grafanaTemplates, alertManagerTemplates, thanosTemplates, proxyTemplates, endpointObservabilityTemplates, prometheusTemplates []*resource.Resource

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

func GetTemplateRenderer() *TemplateRenderer {
	loadTemplateRendererOnce.Do(func() {
		templatesPath, found := os.LookupEnv(TemplatesPathEnvVar)
		if !found {
			log.Fatalf("TEMPLATES_PATH environment variable is required")
		}
		templateRenderer = &TemplateRenderer{
			templatesPath: templatesPath,
			templates:     map[string]resmap.ResMap{},
		}
	})
	return templateRenderer
}

// GetOrLoadGenericTemplates reads base manifest
func (r *TemplateRenderer) GetOrLoadGenericTemplates() ([]*resource.Resource, error) {
	if len(genericTemplates) > 0 {
		return genericTemplates, nil
	}

	basePath := path.Join(r.templatesPath, "base")

	// add observatorium template
	if err := r.AddTemplateFromPath(basePath+"/observatorium", &genericTemplates); err != nil {
		return genericTemplates, err
	}

	// add config template
	if err := r.AddTemplateFromPath(basePath+"/config", &genericTemplates); err != nil {
		return genericTemplates, err
	}

	return genericTemplates, nil
}

// GetOrLoadGrafanaTemplates reads the grafana manifests
func (r *TemplateRenderer) GetOrLoadGrafanaTemplates() ([]*resource.Resource, error) {
	if len(grafanaTemplates) > 0 {
		return grafanaTemplates, nil
	}

	basePath := path.Join(r.templatesPath, "base")

	// add grafana template
	if err := r.AddTemplateFromPath(basePath+"/grafana", &grafanaTemplates); err != nil {
		return grafanaTemplates, err
	}
	return grafanaTemplates, nil
}

// GetOrLoadAlertManagerTemplates reads the alertmanager manifests
func (r *TemplateRenderer) GetOrLoadAlertManagerTemplates() ([]*resource.Resource, error) {
	if len(alertManagerTemplates) > 0 {
		return alertManagerTemplates, nil
	}

	basePath := path.Join(r.templatesPath, "base")

	// add alertmanager template
	if err := r.AddTemplateFromPath(basePath+"/alertmanager", &alertManagerTemplates); err != nil {
		return alertManagerTemplates, err
	}
	return alertManagerTemplates, nil
}

// GetOrLoadThanosTemplates reads the thanos manifests
func (r *TemplateRenderer) GetOrLoadThanosTemplates() ([]*resource.Resource, error) {
	if len(thanosTemplates) > 0 {
		return thanosTemplates, nil
	}

	basePath := path.Join(r.templatesPath, "base")

	// add thanos template
	if err := r.AddTemplateFromPath(basePath+"/thanos", &thanosTemplates); err != nil {
		return thanosTemplates, err
	}
	return thanosTemplates, nil
}

// GetOrLoadProxyTemplates reads the rbac-query-proxy manifests
func (r *TemplateRenderer) GetOrLoadProxyTemplates() ([]*resource.Resource, error) {
	if len(proxyTemplates) > 0 {
		return proxyTemplates, nil
	}

	basePath := path.Join(r.templatesPath, "base")

	// add rbac-query-proxy template
	if err := r.AddTemplateFromPath(basePath+"/proxy", &proxyTemplates); err != nil {
		return proxyTemplates, err
	}
	return proxyTemplates, nil
}

// GetEndpointObservabilityTemplates reads endpoint-observability manifest
func (r *TemplateRenderer) GetOrLoadEndpointObservabilityTemplates() ([]*resource.Resource, error) {
	if len(endpointObservabilityTemplates) > 0 {
		return endpointObservabilityTemplates, nil
	}

	basePath := path.Join(r.templatesPath, "endpoint-observability")

	// add endpoint ovservability template
	if err := r.AddTemplateFromPath(basePath, &endpointObservabilityTemplates); err != nil {
		return endpointObservabilityTemplates, err
	}

	return endpointObservabilityTemplates, nil
}

// GetOrLoadPrometheusTemplates reads endpoint-observability manifest
func (r *TemplateRenderer) GetOrLoadPrometheusTemplates() ([]*resource.Resource, error) {
	if len(prometheusTemplates) > 0 {
		return prometheusTemplates, nil
	}

	basePath := path.Join(r.templatesPath, "prometheus")

	// add endpoint ovservability template
	if err := r.AddTemplateFromPath(basePath, &prometheusTemplates); err != nil {
		return prometheusTemplates, err
	}

	return prometheusTemplates, nil
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

// ResetTemplates reset all the loaded templates
func (r *TemplateRenderer) ResetTemplates() {
	genericTemplates = nil
	grafanaTemplates = nil
	alertManagerTemplates = nil
	thanosTemplates = nil
	proxyTemplates = nil
	endpointObservabilityTemplates = nil
}
