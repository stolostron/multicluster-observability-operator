package util

import (
	"encoding/json"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/rendering/templates"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/v3/pkg/resource"
)

const (
	Path = "/usr/local/endpoint-metrics-operator-template"
)

func loadTemplates() ([]runtime.RawExtension, error) {
	templateRenderer := templates.NewTemplateRenderer(Path)
	resourceList := []*resource.Resource{}
	templateRenderer.AddTemplateFromPath(Path, &resourceList)
	rawExtensionList := []runtime.RawExtension{}
	for _, r := range resourceList {
		r.SetNamespace(SpokeNameSpace)
		rJson, err := json.Marshal(r.Map())
		if err != nil {
			return nil, err
		}
		log.Info("rJson", "string", string(rJson))
		rawExtensionList = append(rawExtensionList, runtime.RawExtension{Raw: rJson})
	}
	return rawExtensionList, nil
}
