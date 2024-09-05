// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/api/resource"

	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
)

const (
	cloSubscriptionChannel = "stable-5.9"
)

func (r *MCORenderer) newCLORenderer() {
	r.renderCLOFns = map[string]rendererutil.RenderFn{
		"Namespace":     r.renderer.RenderNamespace,
		"Subscription":  r.renderSubscription,
		"OperatorGroup": r.renderOperatorGroup,
	}
}

func (r *MCORenderer) renderSubscription(
	res *resource.Resource,
	namespace string,
	labels map[string]string,
) (*unstructured.Unstructured, error) {
	m, err := res.Map()
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{Object: m}

	cLabels := u.GetLabels()
	if cLabels == nil {
		cLabels = make(map[string]string)
	}
	for k, v := range labels {
		cLabels[k] = v
	}
	u.SetLabels(cLabels)

	return u, nil
}
func (r *MCORenderer) renderOperatorGroup(
	res *resource.Resource,
	namespace string,
	labels map[string]string,
) (*unstructured.Unstructured, error) {
	m, err := res.Map()
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{Object: m}

	cLabels := u.GetLabels()
	if cLabels == nil {
		cLabels = make(map[string]string)
	}
	for k, v := range labels {
		cLabels[k] = v
	}
	u.SetLabels(cLabels)

	return u, nil
}

func (r *MCORenderer) renderCLOTemplates(
	templates []*resource.Resource,
	namespace string,
	labels map[string]string,
) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderMCOAFns[template.GetKind()]
		if !ok {
			m, err := template.Map()
			if err != nil {
				return []*unstructured.Unstructured{}, err
			}
			uobjs = append(uobjs, &unstructured.Unstructured{Object: m})
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
