// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/v3/pkg/resource"

	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/rendering/templates"
)

const (
	templatePath = "/usr/local/endpoint-metrics-operator-template"
)

func loadTemplates() ([]runtime.RawExtension, error) {
	templateRenderer := templates.NewTemplateRenderer(templatePath)
	resourceList := []*resource.Resource{}
	err := templateRenderer.AddTemplateFromPath(templatePath, &resourceList)
	if err != nil {
		log.Error(err, "Failed to load templates")
		return nil, err
	}
	rawExtensionList := []runtime.RawExtension{}
	for _, r := range resourceList {
		r.SetNamespace(spokeNameSpace)
		rJson, err := json.Marshal(r.Map())
		if err != nil {
			return nil, err
		}
		log.Info("rJson", "string", string(rJson))
		rawExtensionList = append(rawExtensionList, runtime.RawExtension{Raw: rJson})
	}
	return rawExtensionList, nil
}
