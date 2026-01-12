// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"errors"
	"maps"
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/api/resource"
)

const (
	metadataErr     = "failed to find metadata field"
	nsUpdateAnnoKey = "update-namespace"
)

type RenderFn func(*resource.Resource, string, map[string]string) (*unstructured.Unstructured, error)

type Renderer struct {
	renderFns map[string]RenderFn
}

func NewRenderer() *Renderer {
	renderer := &Renderer{}
	renderer.renderFns = map[string]RenderFn{
		"Deployment":               renderer.RenderDeployments,
		"StatefulSet":              renderer.RenderNamespace,
		"DaemonSet":                renderer.RenderNamespace,
		"Service":                  renderer.RenderNamespace,
		"ServiceAccount":           renderer.RenderNamespace,
		"ConfigMap":                renderer.RenderNamespace,
		"ClusterRole":              renderer.RenderClusterRole,
		"ClusterRoleBinding":       renderer.RenderClusterRoleBinding,
		"Secret":                   renderer.RenderNamespace,
		"Role":                     renderer.RenderNamespace,
		"RoleBinding":              renderer.RenderNamespace,
		"Ingress":                  renderer.RenderNamespace,
		"PersistentVolumeClaim":    renderer.RenderNamespace,
		"Prometheus":               renderer.RenderNamespace,
		"PrometheusRule":           renderer.RenderNamespace,
		"CustomResourceDefinition": renderer.RenderNamespace,
	}
	return renderer
}

func (r *Renderer) RenderTemplates(
	templates []*resource.Resource,
	namespace string,
	labels map[string]string,
) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderFns[template.GetKind()]
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

func (r *Renderer) RenderDeployments(
	res *resource.Resource,
	namespace string,
	labels map[string]string,
) (*unstructured.Unstructured, error) {
	/* 	err := patching.ApplyGlobalPatches(res, r.cr)
	   	if err != nil {
	   		return nil, err
	   	} */

	if err := res.SetNamespace(namespace); err != nil {
		return nil, err
	}
	m, err := res.Map()
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{Object: m}
	return u, nil
}

func (r *Renderer) RenderNamespace(
	res *resource.Resource,
	namespace string,
	labels map[string]string,
) (*unstructured.Unstructured, error) {
	m, err := res.Map()
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{Object: m}
	if UpdateNamespace(u) {
		u.SetNamespace(namespace)
		if err := res.SetNamespace(namespace); err != nil {
			return nil, err
		}
	}

	return u, nil
}

func (r *Renderer) RenderClusterRole(
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
	maps.Copy(cLabels, labels)
	u.SetLabels(cLabels)

	return u, nil
}

func (r *Renderer) RenderClusterRoleBinding(
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
	maps.Copy(cLabels, labels)
	u.SetLabels(cLabels)

	subjects, ok := u.Object["subjects"].([]any)
	if !ok {
		return nil, errors.New("failed to find clusterrolebinding subjects field")
	}
	subject := subjects[0].(map[string]any)
	kind := subject["kind"]
	if kind == "Group" {
		return u, nil
	}

	if UpdateNamespace(u) {
		subject["namespace"] = namespace
		u.SetNamespace(namespace)
	}

	return u, nil
}

// UpdateNamespace checks for annotiation to update NS.
func UpdateNamespace(u *unstructured.Unstructured) bool {
	annotations := u.GetAnnotations()
	v, ok := annotations[nsUpdateAnnoKey]
	if !ok {
		return true
	}
	ret, _ := strconv.ParseBool(v)
	return ret
}
