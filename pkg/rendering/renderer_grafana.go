// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/v3/pkg/resource"

	"github.com/open-cluster-management/multicluster-observability-operator/pkg/util"
)

func (r *Renderer) newGranfanaRenderer() {
	r.renderGrafanaFns = map[string]renderFn{
		"Deployment":            r.renderGrafanaDeployments,
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

func (r *Renderer) renderGrafanaDeployments(res *resource.Resource) (*unstructured.Unstructured, error) {
	u, err := r.renderDeployments(res)
	if err != nil {
		return nil, err
	}

	spec, ok := u.Object["spec"].(map[string]interface{})
	if ok {
		spec["replicas"] = util.GetReplicaCount("Deployment")
	}
	return u, nil
}

func (r *Renderer) renderGrafanaTemplates(templates []*resource.Resource) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderGrafanaFns[template.GetKind()]
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
