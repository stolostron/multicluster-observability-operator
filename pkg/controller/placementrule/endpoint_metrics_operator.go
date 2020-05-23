// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/v3/pkg/resource"

	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/rendering/templates"
)

const (
	templatePath  = "/usr/local/endpoint-metrics-operator-template"
	endpointImage = "quay.io/open-cluster-management/endpoint-metrics-operator:0.1.0"
	deployName    = "endpoint-metrics-operator"
)

func getK8sObj(kind string) runtime.Object {
	objs := map[string]runtime.Object{
		"Deployment":         &v1.Deployment{},
		"ClusterRole":        &rbacv1.ClusterRole{},
		"ClusterRoleBinding": &rbacv1.ClusterRoleBinding{},
		"ServiceAccount":     &corev1.ServiceAccount{},
	}
	return objs[kind]
}

func loadTemplates() ([]runtime.RawExtension, error) {
	templateRenderer := templates.NewTemplateRenderer(templatePath)
	resourceList := []*resource.Resource{}
	err := templateRenderer.AddTemplateFromPath(templatePath, &resourceList)
	if err != nil {
		log.Error(err, "Failed to load templates")
		return nil, err
	}
	rawExtensionList := []runtime.RawExtension{}
	for _, r := range resourceList {
		r.SetNamespace(spokeNameSpace)
		obj := getK8sObj(r.GetKind())
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(r.Map(), obj)
		if err != nil {
			log.Error(err, "failed to convert the resource", r.GetName())
			return nil, err
		}

		// set the image for endpoint metrics operator
		if r.GetKind() == "Deployment" && r.GetName() == deployName {
			spec := obj.(*v1.Deployment).Spec.Template.Spec
			spec.Containers[0].Image = endpointImage
		}

		rawExtensionList = append(rawExtensionList, runtime.RawExtension{Object: obj})
	}
	return rawExtensionList, nil
}
