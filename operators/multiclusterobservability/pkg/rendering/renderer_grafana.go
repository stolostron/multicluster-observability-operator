// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"encoding/json"
	"fmt"
	"strings"

	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/api/resource"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
)

const dashboardFolderAnnotationKey = "observability.open-cluster-management.io/dashboard-folder"

func (r *MCORenderer) newGranfanaRenderer() {
	r.renderGrafanaFns = map[string]rendererutil.RenderFn{
		"Deployment":            r.renderGrafanaDeployments,
		"Service":               r.renderer.RenderNamespace,
		"ServiceAccount":        r.renderer.RenderNamespace,
		"ConfigMap":             r.renderer.RenderNamespace,
		"ClusterRole":           r.renderer.RenderClusterRole,
		"ClusterRoleBinding":    r.renderer.RenderClusterRoleBinding,
		"Secret":                r.renderer.RenderNamespace,
		"Role":                  r.renderer.RenderNamespace,
		"RoleBinding":           r.renderer.RenderNamespace,
		"Ingress":               r.renderer.RenderNamespace,
		"PersistentVolumeClaim": r.renderer.RenderNamespace,
		"ScrapeConfig":          r.renderer.RenderNamespace,
		"PrometheusRule":        r.renderer.RenderNamespace,
	}
}

func (r *MCORenderer) renderGrafanaDeployments(res *resource.Resource,
	namespace string, labels map[string]string) (*unstructured.Unstructured, error) {
	u, err := r.renderer.RenderDeployments(res, namespace, labels)
	if err != nil {
		return nil, err
	}

	obj := util.GetK8sObj(u.GetKind())
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj)
	if err != nil {
		return nil, err
	}
	dep := obj.(*v1.Deployment)
	dep.Name = config.GetOperandName(config.Grafana)
	dep.Spec.Replicas = config.GetReplicas(config.Grafana, r.cr.Spec.InstanceSize, r.cr.Spec.AdvancedConfig)

	spec := &dep.Spec.Template.Spec
	imagePullPolicy := config.GetImagePullPolicy(r.cr.Spec)

	spec.Containers[0].Image = config.DefaultImgRepository + "/" + config.GrafanaImgKey +
		":" + config.DefaultImgTagSuffix
	found, image := config.ReplaceImage(r.cr.Annotations, spec.Containers[0].Image, config.GrafanaImgKey)
	if found {
		spec.Containers[0].Image = image
	}
	spec.Containers[0].ImagePullPolicy = imagePullPolicy
	spec.Containers[0].Resources = config.GetResources(config.Grafana, r.cr.Spec.InstanceSize, r.cr.Spec.AdvancedConfig)

	spec.Containers[1].Image = config.DefaultImgRepository + "/" + config.GrafanaDashboardLoaderName +
		":" + config.DefaultImgTagSuffix
	found, image = config.ReplaceImage(r.cr.Annotations, spec.Containers[1].Image,
		config.GrafanaDashboardLoaderKey)
	if found {
		spec.Containers[1].Image = image
	}
	spec.Containers[1].ImagePullPolicy = imagePullPolicy

	// If we're on OCP and has imagestreams, we always want the oauth image
	// from the imagestream, and fail the reconcile if we don't find it.
	// If we're on non-OCP (tests) we use the base template image
	found, image = config.GetOauthProxyImage(r.imageClient)
	if found {
		spec.Containers[2].Image = image
	} else if r.HasImagestream() {
		return nil, fmt.Errorf("failed to get OAuth image for alertmanager")
	}
	spec.Containers[2].ImagePullPolicy = imagePullPolicy

	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	return &unstructured.Unstructured{Object: unstructuredObj}, nil
}

func (r *MCORenderer) renderGrafanaTemplates(templates []*resource.Resource,
	namespace string, labels map[string]string) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		// Avoid rendering resource kinds that are specific to MCOA.
		if !MCOAPlatformMetricsEnabled(r.cr) && isMCOASpecificResourceKind(template) {
			continue
		}

		template = template.DeepCopy()

		// Add deprecated suffix to the old dashboard names when MCOA is activated.
		if MCOAPlatformMetricsEnabled(r.cr) && isNonMCOASpecificDashboard(template) {
			if err := addDeprecatedSuffixToDashboardName(template); err != nil {
				return []*unstructured.Unstructured{}, fmt.Errorf("failed to modify dashboard title with deprecated suffix: %w", err)
			}
		}

		render, ok := r.renderGrafanaFns[template.GetKind()]
		if !ok {
			m, err := template.Map()
			if err != nil {
				return []*unstructured.Unstructured{}, err
			}
			uobjs = append(uobjs, &unstructured.Unstructured{Object: m})
			continue
		}
		uobj, err := render(template, namespace, labels)
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

// isMCOASpecificResourceType returns true if the resource is a scrape config or a Prometheus rule
func isMCOASpecificResourceKind(res *resource.Resource) bool {
	kind := res.GetKind()
	if kind == "ScrapeConfig" || kind == "PrometheusRule" {
		return true
	}

	return false
}

func isNonMCOASpecificDashboard(res *resource.Resource) bool {
	// Exclude all dashboards living in the default directory as they are all duplicated
	// for MCOA with some expressions adaptations due to the different set of metrics
	// being collected.
	if res.GetKind() == "ConfigMap" {
		dir, ok := res.GetAnnotations(dashboardFolderAnnotationKey)[dashboardFolderAnnotationKey]
		if !ok || dir == "" {
			return true
		}

		if strings.Contains(dir, "OCP 3.11") {
			return true
		}
	}

	return false
}

func addDeprecatedSuffixToDashboardName(template *resource.Resource) error {
	dataMap := template.GetDataMap()
	if len(dataMap) != 1 {
		return fmt.Errorf("unexpected number of values in the dashboard configmap %s: %d", template.GetName(), len(dataMap))
	}

	var key, val string
	for k, v := range dataMap {
		key, val = k, v
	}

	dashboard := map[string]interface{}{}
	if err := json.Unmarshal([]byte(val), &dashboard); err != nil {
		return fmt.Errorf("failed to unmarshal dashboard from configmap %q: %w", template.GetName(), err)
	}

	title, ok := dashboard["title"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid 'title' field in dashboard %q", key)
	}
	dashboard["title"] = title + " - DEPRECATED"

	newVal, err := json.Marshal(dashboard)
	if err != nil {
		return fmt.Errorf("failed to marshal modified dashboard %q: %w", template.GetName(), err)
	}

	dataMap[key] = string(newVal)
	template.SetDataMap(dataMap)

	return nil
}
