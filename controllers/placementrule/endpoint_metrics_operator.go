// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/v3/pkg/resource"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/rendering/templates"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/util"
)

const (
	deployName      = "endpoint-observability-operator"
	saName          = "endpoint-observability-operator-sa"
	rolebindingName = "open-cluster-management:endpoint-observability-operator-rb"
)

var (
	templatePath = "/usr/local/manifests/endpoint-observability"
)

func loadTemplates(namespace string,
	mco *mcov1beta2.MultiClusterObservability) ([]runtime.RawExtension, error) {
	templateRenderer := templates.NewTemplateRenderer(templatePath)
	resourceList := []*resource.Resource{}
	err := templateRenderer.AddTemplateFromPath(templatePath, &resourceList)
	if err != nil {
		log.Error(err, "Failed to load templates")
		return nil, err
	}
	rawExtensionList := []runtime.RawExtension{}
	for _, r := range resourceList {
		if r.GetKind() == "CustomResourceDefinition" {
			data, err := r.MarshalJSON()
			if err != nil {
				return nil, err
			}
			rawExtensionList = append(rawExtensionList, runtime.RawExtension{Raw: data})
		} else {
			obj, err := updateRes(r, namespace, mco)
			if err != nil {
				return nil, err
			}
			rawExtensionList = append(rawExtensionList, runtime.RawExtension{Object: obj})
		}
	}
	return rawExtensionList, nil
}

func updateRes(r *resource.Resource, namespace string,
	mco *mcov1beta2.MultiClusterObservability) (runtime.Object, error) {

	kind := r.GetKind()
	if kind != "ClusterRole" && kind != "ClusterRoleBinding" {
		r.SetNamespace(spokeNameSpace)
	}
	obj := util.GetK8sObj(kind)
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(r.Map(), obj)
	if err != nil {
		log.Error(err, "failed to convert the resource", "resource", r.GetName())
		return nil, err
	}
	// set the images and watch_namespace for endpoint metrics operator
	if r.GetKind() == "Deployment" && r.GetName() == deployName {
		spec := obj.(*v1.Deployment).Spec.Template.Spec
		for i, container := range spec.Containers {
			if container.Name == "endpoint-observability-operator" {
				spec.Containers[i] = updateEndpointOperator(mco, namespace, container)
			}
			if container.Name == "observability-lease-controller" {
				spec.Containers[i] = updateLeaseController(mco, namespace, container)
			}
		}
	}
	// set the imagepullsecrets for sa
	if r.GetKind() == "ServiceAccount" && r.GetName() == saName {
		imageSecrets := obj.(*corev1.ServiceAccount).ImagePullSecrets
		for i, imageSecret := range imageSecrets {
			if imageSecret.Name == "REPLACE_WITH_IMAGEPULLSECRET" {
				imageSecrets[i].Name = mco.Spec.ImagePullSecret
				break
			}
		}
	}
	// set namespace for rolebinding
	if r.GetKind() == "ClusterRoleBinding" && r.GetName() == rolebindingName {
		binding := obj.(*rbacv1.ClusterRoleBinding)
		binding.Subjects[0].Namespace = spokeNameSpace
	}

	return obj, nil
}

func updateEndpointOperator(mco *mcov1beta2.MultiClusterObservability,
	namespace string, container corev1.Container) corev1.Container {
	container.Image = getImage(mco, mcoconfig.EndpointControllerImgName,
		mcoconfig.EndpointControllerImgTagSuffix, mcoconfig.EndpointControllerKey)
	container.ImagePullPolicy = mco.Spec.ImagePullPolicy
	for i, env := range container.Env {
		if env.Name == "HUB_NAMESPACE" {
			container.Env[i].Value = namespace
		}
		if env.Name == "COLLECTOR_IMAGE" {
			container.Env[i].Value = getImage(mco, mcoconfig.MetricsCollectorImgName,
				mcoconfig.MetricsCollectorImgTagSuffix, mcoconfig.MetricsCollectorKey)
		}
	}
	return container
}

func updateLeaseController(mco *mcov1beta2.MultiClusterObservability,
	namespace string, container corev1.Container) corev1.Container {
	container.Image = getImage(mco, mcoconfig.LeaseControllerImageName,
		mcoconfig.LeaseControllerImageTagSuffix, mcoconfig.LeaseControllerKey)
	container.ImagePullPolicy = mco.Spec.ImagePullPolicy
	for i, arg := range container.Args {
		if arg == "-lease-name" {
			container.Args[i+1] = leaseName
		}
		if arg == "-lease-namespace" {
			container.Args[i+1] = namespace
		}
	}
	return container
}

func getImage(mco *mcov1beta2.MultiClusterObservability,
	name, tag, key string) string {
	image := mcoconfig.DefaultImgRepository +
		"/" + name + ":" + tag
	found, replacedImage := mcoconfig.ReplaceImage(mco.Annotations, image, key)
	if found {
		return replacedImage
	}
	return image
}
