// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/v3/pkg/resource"
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
		"Deployment":            renderer.RenderDeployments,
		"StatefulSet":           renderer.RenderNamespace,
		"DaemonSet":             renderer.RenderNamespace,
		"Service":               renderer.RenderNamespace,
		"ServiceAccount":        renderer.RenderNamespace,
		"ConfigMap":             renderer.RenderNamespace,
		"ClusterRole":           renderer.RenderClusterRole,
		"ClusterRoleBinding":    renderer.RenderClusterRoleBinding,
		"Secret":                renderer.RenderNamespace,
		"Role":                  renderer.RenderNamespace,
		"RoleBinding":           renderer.RenderNamespace,
		"Ingress":               renderer.RenderNamespace,
		"PersistentVolumeClaim": renderer.RenderNamespace,
	}
	return renderer
}

func (r *Renderer) RenderTemplates(templates []*resource.Resource, namespace string, labels map[string]string) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderFns[template.GetKind()]
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

func (r *Renderer) RenderDeployments(res *resource.Resource, namespace string, labels map[string]string) (*unstructured.Unstructured, error) {
	/* 	err := patching.ApplyGlobalPatches(res, r.cr)
	   	if err != nil {
	   		return nil, err
	   	} */

	res.SetNamespace(namespace)
	u := &unstructured.Unstructured{Object: res.Map()}
	return u, nil
}

func (r *Renderer) RenderNamespace(res *resource.Resource, namespace string, labels map[string]string) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}
	if UpdateNamespace(u) {
		res.SetNamespace(namespace)
	}

	return u, nil
}

func (r *Renderer) RenderClusterRole(res *resource.Resource, namespace string, labels map[string]string) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}

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

func (r *Renderer) RenderClusterRoleBinding(res *resource.Resource, namespace string, labels map[string]string) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}

	cLabels := u.GetLabels()
	if cLabels == nil {
		cLabels = make(map[string]string)
	}
	for k, v := range labels {
		cLabels[k] = v
	}
	u.SetLabels(cLabels)

	subjects, ok := u.Object["subjects"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to find clusterrolebinding subjects field")
	}
	subject := subjects[0].(map[string]interface{})
	kind := subject["kind"]
	if kind == "Group" {
		return u, nil
	}

	if UpdateNamespace(u) {
		subject["namespace"] = namespace
	}

	return u, nil
}

// UpdateNamespace checks for annotiation to update NS
func UpdateNamespace(u *unstructured.Unstructured) bool {
	annotations := u.GetAnnotations()
	v, ok := annotations[nsUpdateAnnoKey]
	if !ok {
		return true
	}
	ret, _ := strconv.ParseBool(v)
	return ret
}
