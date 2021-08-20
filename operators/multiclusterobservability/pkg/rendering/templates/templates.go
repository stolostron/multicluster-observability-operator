// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package templates

import (
	"path"

	"sigs.k8s.io/kustomize/v3/pkg/resource"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering/templates"
)

// GetGrafanaTemplates reads the grafana manifests
func GetGrafanaTemplates(r *templates.TemplateRenderer,
	mco *mcov1beta2.MultiClusterObservability) ([]*resource.Resource, error) {
	basePath := path.Join(r.GetTemplatesPath(), "base")
	// resourceList contains all kustomize resources
	resourceList := []*resource.Resource{}

	// add grafana template
	if err := r.AddTemplateFromPath(basePath+"/grafana", &resourceList); err != nil {
		return resourceList, err
	}
	return resourceList, nil
}

// GetAlertManagerTemplates reads the alertmanager manifests
func GetAlertManagerTemplates(r *templates.TemplateRenderer,
	mco *mcov1beta2.MultiClusterObservability) ([]*resource.Resource, error) {
	basePath := path.Join(r.GetTemplatesPath(), "base")
	// resourceList contains all kustomize resources
	resourceList := []*resource.Resource{}

	// add alertmanager template
	if err := r.AddTemplateFromPath(basePath+"/alertmanager", &resourceList); err != nil {
		return resourceList, err
	}
	return resourceList, nil
}

// GetThanosTemplates reads the thanos manifests
func GetThanosTemplates(r *templates.TemplateRenderer,
	mco *mcov1beta2.MultiClusterObservability) ([]*resource.Resource, error) {
	basePath := path.Join(r.GetTemplatesPath(), "base")
	// resourceList contains all kustomize resources
	resourceList := []*resource.Resource{}

	// add thanos template
	if err := r.AddTemplateFromPath(basePath+"/thanos", &resourceList); err != nil {
		return resourceList, err
	}
	return resourceList, nil
}

// GetProxyTemplates reads the rbac-query-proxy manifests
func GetProxyTemplates(r *templates.TemplateRenderer,
	mco *mcov1beta2.MultiClusterObservability) ([]*resource.Resource, error) {
	basePath := path.Join(r.GetTemplatesPath(), "base")
	// resourceList contains all kustomize resources
	resourceList := []*resource.Resource{}

	// add rbac-query-proxy template
	if err := r.AddTemplateFromPath(basePath+"/proxy", &resourceList); err != nil {
		return resourceList, err
	}
	return resourceList, nil
}

// GetTemplates reads base manifest
func GetTemplates(r *templates.TemplateRenderer,
	mco *mcov1beta2.MultiClusterObservability) ([]*resource.Resource, error) {
	basePath := path.Join(r.GetTemplatesPath(), "base")
	// resourceList contains all kustomize resources
	resourceList := []*resource.Resource{}

	// add observatorium template
	if err := r.AddTemplateFromPath(basePath+"/observatorium", &resourceList); err != nil {
		return resourceList, err
	}

	// add config template
	if err := r.AddTemplateFromPath(basePath+"/config", &resourceList); err != nil {
		return resourceList, err
	}

	return resourceList, nil
}
