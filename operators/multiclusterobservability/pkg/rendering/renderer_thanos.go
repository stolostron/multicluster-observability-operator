// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/v3/pkg/resource"

	rendererutil "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering"
)

func (r *MCORenderer) newThanosRenderer() {
	r.renderThanosFns = map[string]rendererutil.RenderFn{
		"ServiceAccount":     r.renderer.RenderNamespace,
		"ConfigMap":          r.renderer.RenderNamespace,
		"ClusterRole":        r.renderer.RenderClusterRole,
		"ClusterRoleBinding": r.renderer.RenderClusterRoleBinding,
		"Secret":             r.renderer.RenderNamespace,
	}
}

func (r *MCORenderer) renderThanosTemplates(templates []*resource.Resource,
	namespace string, labels map[string]string) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderThanosFns[template.GetKind()]
		if !ok {
			uobjs = append(uobjs, &unstructured.Unstructured{Object: template.Map()})
			continue
		}
		uobj, err := render(template.DeepCopy(), namespace, labels)
		if err != nil {
			return []*unstructured.Unstructured{}, err
		}
		if uobj == nil {
			continue
		}
		uobjs = append(uobjs, uobj)

	}

	return uobjs, nil
}
