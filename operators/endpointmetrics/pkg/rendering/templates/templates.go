// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package templates

import (
	"sigs.k8s.io/kustomize/api/resource"

	"github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
)

// GetTemplates reads base manifest
func GetTemplates(r *templates.TemplateRenderer) ([]*resource.Resource, error) {

	// resourceList contains all kustomize resources
	resourceList := []*resource.Resource{}

	// add prometheus template
	if err := r.AddTemplateFromPath(r.GetTemplatesPath()+"/prometheus/crd", &resourceList); err != nil {
		return resourceList, err
	}

	// add prometheus template
	if err := r.AddTemplateFromPath(r.GetTemplatesPath()+"/prometheus", &resourceList); err != nil {
		return resourceList, err
	}

	// add prometheus template
	if err := r.AddTemplateFromPath(r.GetTemplatesPath()+"/prometheus/prometheusrules", &resourceList); err != nil {
		return resourceList, err
	}

	return resourceList, nil
}
