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

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
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

// GetGrafanaTemplates reads the grafana manifests
func (r *TemplateRenderer) GetGrafanaTemplates(mcm *monitoringv1alpha1.MultiClusterMonitoring) ([]*resource.Resource, error) {
	basePath := path.Join(r.templatesPath, "base")
	// resourceList contains all kustomize resources
	resourceList := []*resource.Resource{}

	// add grafana template
	if err := r.addTemplateFromPath(basePath+"/grafana", &resourceList); err != nil {
		return resourceList, err
	}
	return resourceList, nil
}

// GetMinioTemplates reads the minio manifests
func (r *TemplateRenderer) GetMinioTemplates(mcm *monitoringv1alpha1.MultiClusterMonitoring) ([]*resource.Resource, error) {
	basePath := path.Join(r.templatesPath, "base")
	// resourceList contains all kustomize resources
	resourceList := []*resource.Resource{}

	if mcm.Spec.ObjectStorageConfigSpec.Type == "minio" {
		// add minio template
		if err := r.addTemplateFromPath(basePath+"/object_storage/minio", &resourceList); err != nil {
			return resourceList, err
		}
	}

	return resourceList, nil
}

// GetTemplates reads base manifest
func (r *TemplateRenderer) GetTemplates(mcm *monitoringv1alpha1.MultiClusterMonitoring) ([]*resource.Resource, error) {
	basePath := path.Join(r.templatesPath, "base")
	// resourceList contains all kustomize resources
	resourceList := []*resource.Resource{}

	// add observatorium template
	if err := r.addTemplateFromPath(basePath+"/observatorium", &resourceList); err != nil {
		return resourceList, err
	}

	objStorageType := mcm.Spec.ObjectStorageConfigSpec.Type
	// add s3 template
	if objStorageType == "s3" {
		if err := r.addTemplateFromPath(basePath+"/object_storage/s3", &resourceList); err != nil {
			return resourceList, err
		}
	}
	return resourceList, nil
}

func (r *TemplateRenderer) addTemplateFromPath(kustomizationPath string, resourceList *[]*resource.Resource) error {
	var err error
	resMap, ok := r.templates[kustomizationPath]
	if !ok {
		resMap, err = r.render(kustomizationPath)
		if err != nil {
			return err
		}
		r.templates[kustomizationPath] = resMap
		*resourceList = append(*resourceList, resMap.Resources()...)
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
