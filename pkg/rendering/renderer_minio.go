// Copyright (c) 2020 Red Hat, Inc.

package rendering

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/v3/pkg/resource"
)

func (r *Renderer) newMinioRenderer() {
	r.renderMinioFns = map[string]renderFn{
		"Deployment":            r.renderMinioDeployments,
		"Service":               r.renderNamespace,
		"ServiceAccount":        r.renderNamespace,
		"ConfigMap":             r.renderNamespace,
		"ClusterRoleBinding":    r.renderClusterRoleBinding,
		"Secret":                r.renderNamespace,
		"Role":                  r.renderNamespace,
		"RoleBinding":           r.renderNamespace,
		"Ingress":               r.renderNamespace,
		"PersistentVolumeClaim": r.renderNamespace,
	}
}

func (r *Renderer) renderMinioDeployments(res *resource.Resource) (*unstructured.Unstructured, error) {
	u, err := r.renderDeployments(res)
	if err != nil {
		return nil, err
	}

	return u, nil
}

func (r *Renderer) renderMinioTemplates(templates []*resource.Resource) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderMinioFns[template.GetKind()]
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
