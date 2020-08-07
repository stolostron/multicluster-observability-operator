// Copyright (c) 2020 Red Hat, Inc.

package rendering

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/kustomize/v3/pkg/resource"

	monitoringv1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	mcoconfig "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/rendering/patching"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/rendering/templates"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

const (
	metadataErr = "failed to find metadata field"

	nsUpdateAnnoKey = "update-namespace"
)

var log = logf.Log.WithName("renderer")

type renderFn func(*resource.Resource) (*unstructured.Unstructured, error)

type Renderer struct {
	cr               *monitoringv1.MultiClusterObservability
	renderFns        map[string]renderFn
	renderGrafanaFns map[string]renderFn
	renderMinioFns   map[string]renderFn
}

func NewRenderer(multipleClusterMonitoring *monitoringv1.MultiClusterObservability) *Renderer {
	renderer := &Renderer{cr: multipleClusterMonitoring}
	renderer.renderFns = map[string]renderFn{
		"Deployment":            renderer.renderDeployments,
		"Service":               renderer.renderNamespace,
		"ServiceAccount":        renderer.renderNamespace,
		"ConfigMap":             renderer.renderNamespace,
		"ClusterRoleBinding":    renderer.renderClusterRoleBinding,
		"Secret":                renderer.renderSecret,
		"Role":                  renderer.renderNamespace,
		"RoleBinding":           renderer.renderNamespace,
		"Ingress":               renderer.renderNamespace,
		"PersistentVolumeClaim": renderer.renderPersistentVolumeClaim,
	}
	renderer.newGranfanaRenderer()
	renderer.newMinioRenderer()
	return renderer
}

func (r *Renderer) Render(c runtimeclient.Client) ([]*unstructured.Unstructured, error) {

	genericTemplates, err := templates.GetTemplateRenderer().GetTemplates(r.cr)
	if err != nil {
		return nil, err
	}
	resources, err := r.renderTemplates(genericTemplates)
	if err != nil {
		return nil, err
	}

	// render grafana templates
	grafanaTemplates, err := templates.GetTemplateRenderer().GetGrafanaTemplates(r.cr)
	if err != nil {
		return nil, err
	}
	grafanaResources, err := r.renderGrafanaTemplates(grafanaTemplates)
	if err != nil {
		return nil, err
	}
	resources = append(resources, grafanaResources...)

	// render grafana templates
	minioTemplates, err := templates.GetTemplateRenderer().GetMinioTemplates(r.cr)
	if err != nil {
		return nil, err
	}
	minioResources, err := r.renderMinioTemplates(minioTemplates)
	if err != nil {
		return nil, err
	}

	return append(resources, minioResources...), nil
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
	err := patching.ApplyGlobalPatches(res, r.cr)
	if err != nil {
		return nil, err
	}

	res.SetNamespace(r.cr.Namespace)

	u := &unstructured.Unstructured{Object: res.Map()}

	metadata, ok := u.Object["metadata"].(map[string]interface{})
	if ok {
		err = replaceInValues(metadata, r.cr)
		if err != nil {
			return nil, err
		}
	}

	spec, ok := u.Object["spec"].(map[string]interface{})
	if ok {
		selector, ok := spec["selector"].(map[string]interface{})
		if ok {
			err = replaceInValues(selector, r.cr)
			if err != nil {
				return nil, err
			}
		}

		template, ok := spec["template"].(map[string]interface{})
		if ok {
			metadata, ok := template["metadata"].(map[string]interface{})
			if ok {
				err = replaceInValues(metadata, r.cr)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return u, nil
}

func (r *Renderer) renderNamespace(res *resource.Resource) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}

	if UpdateNamespace(u) {
		res.SetNamespace(r.cr.Namespace)
	}

	return &unstructured.Unstructured{Object: res.Map()}, nil
}

func (r *Renderer) renderPersistentVolumeClaim(res *resource.Resource) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}

	if UpdateNamespace(u) {
		res.SetNamespace(r.cr.Namespace)
	}

	// Update channel to prepend the CRs namespace
	spec, ok := u.Object["spec"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to find spec field")
	}
	err := replaceInValues(spec, r.cr)
	if err != nil {
		return nil, err
	}

	return u, nil
}

// render object storage secret config
func (r *Renderer) renderSecret(res *resource.Resource) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}

	if UpdateNamespace(u) {
		res.SetNamespace(r.cr.Namespace)
	}

	name := res.GetName()
	switch name {

	case "thanos-objectstorage":
		stringData, ok := u.Object["stringData"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("failed to find stringData field")
		}
		err := replaceInValues(stringData, r.cr)
		if err != nil {
			return nil, err
		}
	}

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
		subject["namespace"] = r.cr.Namespace
	}

	return u, nil
}

func stringValueReplace(toReplace string, cr *monitoringv1.MultiClusterObservability) string {

	replaced := toReplace

	replaced = strings.ReplaceAll(
		replaced,
		"{{IMAGEREPO}}",
		string(util.GetAnnotation(cr, mcoconfig.AnnotationKeyImageRepository)),
	)
	replaced = strings.ReplaceAll(replaced, "{{PULLSECRET}}", string(cr.Spec.ImagePullSecret))
	replaced = strings.ReplaceAll(replaced, "{{NAMESPACE}}", string(cr.Namespace))
	replaced = strings.ReplaceAll(replaced, "{{PULLPOLICY}}", string(cr.Spec.ImagePullPolicy))
	replaced = strings.ReplaceAll(replaced, "{{STORAGECLASS}}", string(cr.Spec.StorageClass))
	replaced = strings.ReplaceAll(replaced, "{{MULTICLUSTEROBSERVABILITY_CR_NAME}}", string(cr.Name))

	return replaced
}

func replaceInValues(values map[string]interface{}, cr *monitoringv1.MultiClusterObservability) error {
	for inKey := range values {
		isPrimitiveType := reflect.TypeOf(values[inKey]).String() == "string" ||
			reflect.TypeOf(values[inKey]).String() == "bool" ||
			reflect.TypeOf(values[inKey]).String() == "int"

		if isPrimitiveType {
			if reflect.TypeOf(values[inKey]).String() == "string" {
				values[inKey] = stringValueReplace(values[inKey].(string), cr)
			} // add other options for other primitives when required
		} else if reflect.TypeOf(values[inKey]).Kind().String() == "slice" {
			stringSlice := values[inKey].([]interface{})
			for i := range stringSlice {
				// assumes only slices of strings, which is OK for now
				stringSlice[i] = stringValueReplace(stringSlice[i].(string), cr)
			}
		} else { // reflect.TypeOf(values[inKey]).Kind().String() == "map"
			inValue, ok := values[inKey].(map[string]interface{})
			if !ok {
				return fmt.Errorf("failed to map values")
			}
			err := replaceInValues(inValue, cr)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// UpdateNamespace checks for annotiation to update NS
func UpdateNamespace(u *unstructured.Unstructured) bool {
	metadata, ok := u.Object["metadata"].(map[string]interface{})
	updateNamespace := true
	if ok {
		annotations, ok := metadata["annotations"].(map[string]interface{})
		if ok && annotations != nil {
			if annotations[nsUpdateAnnoKey] != nil && annotations[nsUpdateAnnoKey].(string) != "" {
				updateNamespace, _ = strconv.ParseBool(annotations[nsUpdateAnnoKey].(string))
			}
		}
	}
	return updateNamespace
}

func (r *Renderer) renderMutatingWebhookConfiguration(res *resource.Resource) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}
	webooks, ok := u.Object["webhooks"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to find webhooks spec field")
	}
	webhook := webooks[0].(map[string]interface{})
	clientConfig := webhook["clientConfig"].(map[string]interface{})
	service := clientConfig["service"].(map[string]interface{})

	service["namespace"] = r.cr.Namespace
	return u, nil
}
