// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"os"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/v3/pkg/resource"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/rendering/templates"
)

const (
	templatePath    = "/usr/local/manifests/endpoint-monitoring"
	deployName      = "endpoint-monitoring-operator"
	saName          = "endpoint-monitoring-operator-sa"
	rolebindingName = "endpoint-monitoring-operator-rb"
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

func loadTemplates(namespace string,
	mcm *monitoringv1alpha1.MultiClusterMonitoring) ([]runtime.RawExtension, error) {
	templateRenderer := templates.NewTemplateRenderer(templatePath)
	resourceList := []*resource.Resource{}
	err := templateRenderer.AddTemplateFromPath(templatePath, &resourceList)
	if err != nil {
		log.Error(err, "Failed to load templates")
		return nil, err
	}
	rawExtensionList := []runtime.RawExtension{}
	for _, r := range resourceList {
		kind := r.GetKind()
		if kind != "ClusterRole" && kind != "ClusterRoleBinding" {
			r.SetNamespace(spokeNameSpace)
		}
		obj := getK8sObj(kind)
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(r.Map(), obj)
		if err != nil {
			log.Error(err, "failed to convert the resource", r.GetName())
			return nil, err
		}

		// set the image and watch_namespace for endpoint metrics operator
		if r.GetKind() == "Deployment" && r.GetName() == deployName {
			spec := obj.(*v1.Deployment).Spec.Template.Spec
			spec.Containers[0].Image = os.Getenv("ENDPOINT_OPERATOR_IMAGE")
			for i, env := range spec.Containers[0].Env {
				if env.Name == "WATCH_NAMESPACE" {
					spec.Containers[0].Env[i].Value = namespace
					break
				}
			}
		}
		// set the imagepullsecrets for sa
		if r.GetKind() == "ServiceAccount" && r.GetName() == saName {
			imageSecrets := obj.(*corev1.ServiceAccount).ImagePullSecrets
			for i, imageSecret := range imageSecrets {
				if imageSecret.Name == "REPLACE_WITH_IMAGEPULLSECRET" {
					imageSecrets[i].Name = mcm.Spec.ImagePullSecret
					break
				}
			}
		}
		// set namespace for rolebinding
		if r.GetKind() == "ClusterRoleBinding" && r.GetName() == rolebindingName {
			binding := obj.(*rbacv1.ClusterRoleBinding)
			binding.Subjects[0].Namespace = spokeNameSpace
		}
		rawExtensionList = append(rawExtensionList, runtime.RawExtension{Object: obj})
	}
	return rawExtensionList, nil
}
