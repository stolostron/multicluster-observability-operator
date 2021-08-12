// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"fmt"
	"os"
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/kustomize/v3/pkg/resource"

	"github.com/open-cluster-management/multicluster-observability-operator/operators/endpointmetrics/pkg/rendering/templates"
)

const (
	metadataErr = "failed to find metadata field"

	nsUpdateAnnoKey = "update-namespace"
)

var (
	namespace = os.Getenv("WATCH_NAMESPACE")
	log       = logf.Log.WithName("renderer")
)

type renderFn func(*resource.Resource) (*unstructured.Unstructured, error)

type Renderer struct {
	renderFns map[string]renderFn
}

func NewRenderer() *Renderer {
	renderer := &Renderer{}
	renderer.renderFns = map[string]renderFn{
		"Deployment":            renderer.renderDeployments,
		"Service":               renderer.renderNamespace,
		"ServiceAccount":        renderer.renderNamespace,
		"ConfigMap":             renderer.renderNamespace,
		"ClusterRole":           renderer.renderClusterRole,
		"ClusterRoleBinding":    renderer.renderClusterRoleBinding,
		"Secret":                renderer.renderNamespace,
		"Role":                  renderer.renderNamespace,
		"RoleBinding":           renderer.renderNamespace,
		"Ingress":               renderer.renderNamespace,
		"PersistentVolumeClaim": renderer.renderNamespace,
	}
	return renderer
}

func (r *Renderer) Render(c runtimeclient.Client) ([]*unstructured.Unstructured, error) {

	genericTemplates, err := templates.GetTemplateRenderer().GetTemplates()
	if err != nil {
		return nil, err
	}
	resources, err := r.renderTemplates(genericTemplates)
	if err != nil {
		return nil, err
	}
	/*
		for idx := range resources {
			if resources[idx].GetKind() == "Deployment" {
				obj := util.GetK8sObj(resources[idx].GetKind())
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(resources[idx].Object, obj)
				if err != nil {
					return nil, err
				}
				crLabelKey := config.GetCrLabelKey()
				dep := obj.(*v1.Deployment)
				dep.ObjectMeta.Labels[crLabelKey] = r.cr.Name
				dep.Spec.Selector.MatchLabels[crLabelKey] = r.cr.Name
				dep.Spec.Template.ObjectMeta.Labels[crLabelKey] = r.cr.Name

				spec := &dep.Spec.Template.Spec
				spec.Containers[0].ImagePullPolicy = mcoconfig.GetImagePullPolicy(r.cr.Spec)
				spec.NodeSelector = r.cr.Spec.NodeSelector
				spec.Tolerations = r.cr.Spec.Tolerations
				spec.ImagePullSecrets = []corev1.LocalObjectReference{
					{Name: mcoconfig.GetImagePullSecret(r.cr.Spec)},
				}

				switch resources[idx].GetName() {

				case "observatorium-operator":
					found, image := mcoconfig.ReplaceImage(r.cr.Annotations, spec.Containers[0].Image,
						mcoconfig.ObservatoriumOperatorImgName)
					if found {
						spec.Containers[0].Image = image
					}
					dep.Name = mcoconfig.GetOperandName(config.ObservatoriumOperator)

				}

				unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
				if err != nil {
					return nil, err
				}

				resources[idx].Object = unstructuredObj
			}
		} */

	return resources, nil
}

func (r *Renderer) renderTemplates(templates []*resource.Resource) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderFns[template.GetKind()]
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

func (r *Renderer) renderDeployments(res *resource.Resource) (*unstructured.Unstructured, error) {

	res.SetNamespace(namespace)
	u := &unstructured.Unstructured{Object: res.Map()}
	return u, nil
}

func (r *Renderer) renderNamespace(res *resource.Resource) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}
	if UpdateNamespace(u) {
		res.SetNamespace(namespace)
	}

	return u, nil
}

func (r *Renderer) renderClusterRole(res *resource.Resource) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}

	return u, nil
}

func (r *Renderer) renderClusterRoleBinding(res *resource.Resource) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}

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
