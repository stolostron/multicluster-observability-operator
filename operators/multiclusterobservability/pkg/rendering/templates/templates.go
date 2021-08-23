// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package templates

import (
	"path"

	"sigs.k8s.io/kustomize/v3/pkg/resource"

	"github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering/templates"
)

// *Templates contains all kustomize resources
var genericTemplates, grafanaTemplates, alertManagerTemplates, thanosTemplates, proxyTemplates, endpointObservabilityTemplates, prometheusTemplates []*resource.Resource

// GetOrLoadGenericTemplates reads base manifest
func GetOrLoadGenericTemplates(r *templates.TemplateRenderer) ([]*resource.Resource, error) {
	if len(genericTemplates) > 0 {
		return genericTemplates, nil
	}

	basePath := path.Join(r.GetTemplatesPath(), "base")

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
func GetOrLoadGrafanaTemplates(r *templates.TemplateRenderer) ([]*resource.Resource, error) {
	if len(grafanaTemplates) > 0 {
		return grafanaTemplates, nil
	}

	basePath := path.Join(r.GetTemplatesPath(), "base")

	// add grafana template
	if err := r.AddTemplateFromPath(basePath+"/grafana", &grafanaTemplates); err != nil {
		return grafanaTemplates, err
	}
	return grafanaTemplates, nil
}

// GetOrLoadAlertManagerTemplates reads the alertmanager manifests
func GetOrLoadAlertManagerTemplates(r *templates.TemplateRenderer) ([]*resource.Resource, error) {
	if len(alertManagerTemplates) > 0 {
		return alertManagerTemplates, nil
	}

	basePath := path.Join(r.GetTemplatesPath(), "base")

	// add alertmanager template
	if err := r.AddTemplateFromPath(basePath+"/alertmanager", &alertManagerTemplates); err != nil {
		return alertManagerTemplates, err
	}
	return alertManagerTemplates, nil
}

// GetOrLoadThanosTemplates reads the thanos manifests
func GetOrLoadThanosTemplates(r *templates.TemplateRenderer) ([]*resource.Resource, error) {
	if len(thanosTemplates) > 0 {
		return thanosTemplates, nil
	}

	basePath := path.Join(r.GetTemplatesPath(), "base")

	// add thanos template
	if err := r.AddTemplateFromPath(basePath+"/thanos", &thanosTemplates); err != nil {
		return thanosTemplates, err
	}
	return thanosTemplates, nil
}

// GetOrLoadProxyTemplates reads the rbac-query-proxy manifests
func GetOrLoadProxyTemplates(r *templates.TemplateRenderer) ([]*resource.Resource, error) {
	if len(proxyTemplates) > 0 {
		return proxyTemplates, nil
	}

	basePath := path.Join(r.GetTemplatesPath(), "base")

	// add rbac-query-proxy template
	if err := r.AddTemplateFromPath(basePath+"/proxy", &proxyTemplates); err != nil {
		return proxyTemplates, err
	}
	return proxyTemplates, nil
}

// GetEndpointObservabilityTemplates reads endpoint-observability manifest
func GetOrLoadEndpointObservabilityTemplates(r *templates.TemplateRenderer) ([]*resource.Resource, error) {
	if len(endpointObservabilityTemplates) > 0 {
		return endpointObservabilityTemplates, nil
	}

	basePath := path.Join(r.GetTemplatesPath(), "endpoint-observability")

	// add endpoint ovservability template
	if err := r.AddTemplateFromPath(basePath, &endpointObservabilityTemplates); err != nil {
		return endpointObservabilityTemplates, err
	}

	return endpointObservabilityTemplates, nil
}

// GetOrLoadPrometheusTemplates reads endpoint-observability manifest
func GetOrLoadPrometheusTemplates(r *templates.TemplateRenderer) ([]*resource.Resource, error) {
	if len(prometheusTemplates) > 0 {
		return prometheusTemplates, nil
	}

	basePath := path.Join(r.GetTemplatesPath(), "prometheus")

	// add endpoint ovservability template
	if err := r.AddTemplateFromPath(basePath, &prometheusTemplates); err != nil {
		return prometheusTemplates, err
	}

	return prometheusTemplates, nil
}

// ResetTemplates reset all the loaded templates
func ResetTemplates() {
	genericTemplates = nil
	grafanaTemplates = nil
	alertManagerTemplates = nil
	thanosTemplates = nil
	proxyTemplates = nil
	endpointObservabilityTemplates = nil
}
