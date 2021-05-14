// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/v3/pkg/resource"
)

func (r *Renderer) newThanosRenderer() {
	r.renderThanosFns = map[string]renderFn{
		"ServiceAccount":     r.renderNamespace,
		"ConfigMap":          r.renderNamespace,
		"ClusterRole":        r.renderClusterRole,
		"ClusterRoleBinding": r.renderClusterRoleBinding,
		"Secret":             r.renderNamespace,
	}
}

func (r *Renderer) renderThanosTemplates(templates []*resource.Resource) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderThanosFns[template.GetKind()]
		if !ok {
			uobjs = append(uobjs, &unstructured.Unstructured{Object: template.Map()})
			continue
		}
		uobj, err := render(template.DeepCopy())
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
