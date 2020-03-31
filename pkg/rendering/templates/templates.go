package templates

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
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

	monitoringv1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1"
)

const TemplatesPathEnvVar = "TEMPLATES_PATH"

var loadTemplateRendererOnce sync.Once
var templateRenderer *TemplateRenderer

type TemplateRenderer struct {
	templatesPath string
	templates     map[string]resmap.ResMap
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

func (r *TemplateRenderer) GetTemplates(multiclustermonitoring *monitoringv1.MultiClusterMonitoring) ([]*resource.Resource, error) {
	var err error
	kind := strings.ToLower(multiclustermonitoring.Kind)
	version := multiclustermonitoring.Spec.Version
	key := fmt.Sprintf("%s-%s", kind, version)
	resMap, ok := r.templates[key]
	if !ok {
		resMap, err = r.render(path.Join(r.templatesPath, kind, "version", version))
		if err != nil {
			return nil, err
		}
		r.templates[key] = resMap
	}
	return resMap.Resources(), err
}

func (r *TemplateRenderer) render(kustomizationPath string) (resmap.ResMap, error) {
	ldr, err := loader.NewLoader(loader.RestrictionRootOnly, validator.NewKustValidator(), kustomizationPath, fs.MakeFsOnDisk())
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
