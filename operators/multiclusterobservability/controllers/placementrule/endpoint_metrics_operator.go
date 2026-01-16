// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"fmt"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering/templates"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/api/resource"
)

const (
	deployName      = "endpoint-observability-operator"
	saName          = "endpoint-observability-operator-sa"
	rolebindingName = "open-cluster-management:endpoint-observability-operator-rb"

	kindDeployment               = "Deployment"
	kindConfigMap                = "ConfigMap"
	kindCustomResourceDefinition = "CustomResourceDefinition"
	kindClusterRoleBinding       = "ClusterRoleBinding"
)

// loadTemplates load manifests from manifests directory.
func loadTemplates(mco *mcov1beta2.MultiClusterObservability) (
	[]runtime.RawExtension,
	*apiextensionsv1.CustomResourceDefinition,
	*apiextensionsv1beta1.CustomResourceDefinition,
	*appsv1.Deployment,
	*corev1.ConfigMap,
	error,
) {
	// render endpoint-observability templates
	endpointObsTemplates, err := templates.GetOrLoadEndpointObservabilityTemplates(templatesutil.GetTemplateRenderer())
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to load endpoint observability templates: %w", err)
	}

	crdv1 := &apiextensionsv1.CustomResourceDefinition{}
	crdv1beta1 := &apiextensionsv1beta1.CustomResourceDefinition{}
	dep := &appsv1.Deployment{}
	imageListCM := &corev1.ConfigMap{}
	rawExtensionList := []runtime.RawExtension{}
	for _, r := range endpointObsTemplates {
		obj, err := updateRes(r, mco)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("failed to edit templates: %w", err)
		}
		switch {
		case r.GetKind() == kindDeployment:
			dep = obj.(*appsv1.Deployment)
		case r.GetKind() == kindConfigMap && r.GetName() == operatorconfig.ImageConfigMap:
			imageListCM = obj.(*corev1.ConfigMap)
		case r.GetKind() == kindCustomResourceDefinition:
			if r.GetGvk().Version == "v1" {
				crdv1 = obj.(*apiextensionsv1.CustomResourceDefinition)
			} else {
				crdv1beta1 = obj.(*apiextensionsv1beta1.CustomResourceDefinition)
			}
		default:
			rawExtensionList = append(rawExtensionList, runtime.RawExtension{Object: obj})
		}
	}
	return rawExtensionList, crdv1, crdv1beta1, dep, imageListCM, nil
}

func updateRes(r *resource.Resource,
	mco *mcov1beta2.MultiClusterObservability,
) (runtime.Object, error) {
	kind := r.GetKind()

	if kind != "ClusterRole" && kind != kindClusterRoleBinding && kind != kindCustomResourceDefinition {
		if err := r.SetNamespace(spokeNameSpace); err != nil {
			log.Error(err, "failed to set namespace")
			return nil, err
		}
	}
	obj := util.GetK8sObj(kind)
	if kind == kindCustomResourceDefinition && r.GetGvk().Version == "v1beta1" {
		obj = &apiextensionsv1beta1.CustomResourceDefinition{}
	}
	obj.GetObjectKind()
	m, err := r.Map()
	if err != nil {
		return nil, err
	}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(m, obj)
	if err != nil {
		log.Error(err, "failed to convert the resource", "resource", r.GetName())
		return nil, err
	}
	// set the images and watch_namespace for endpoint metrics operator
	if r.GetKind() == kindDeployment && r.GetName() == deployName {
		spec := obj.(*appsv1.Deployment).Spec.Template.Spec
		for i, container := range spec.Containers {
			if container.Name == "endpoint-observability-operator" {
				spec.Containers[i] = updateEndpointOperator(mco, container)
			}
		}
	}
	// set the imagepullsecrets for sa
	if r.GetKind() == "ServiceAccount" && r.GetName() == saName {
		imageSecrets := obj.(*corev1.ServiceAccount).ImagePullSecrets
		for i, imageSecret := range imageSecrets {
			if imageSecret.Name == "REPLACE_WITH_IMAGEPULLSECRET" {
				imageSecrets[i].Name = mcoconfig.GetImagePullSecret(mco.Spec)
				break
			}
		}
	}
	// set namespace for rolebinding
	if r.GetKind() == kindClusterRoleBinding && r.GetName() == rolebindingName {
		binding := obj.(*rbacv1.ClusterRoleBinding)
		binding.Subjects[0].Namespace = spokeNameSpace
	}
	// set images for components in managed clusters
	if r.GetKind() == kindConfigMap && r.GetName() == operatorconfig.ImageConfigMap {
		images := obj.(*corev1.ConfigMap).Data
		for key := range images {
			found, image := mcoconfig.ReplaceImage(mco.Annotations, images[key], key)
			if found {
				obj.(*corev1.ConfigMap).Data[key] = image
			}
		}
	}

	return obj, nil
}

func updateEndpointOperator(mco *mcov1beta2.MultiClusterObservability,
	container corev1.Container,
) corev1.Container {
	container.Image = getImage(mco, mcoconfig.EndpointControllerImgName,
		mcoconfig.DefaultImgTagSuffix, mcoconfig.EndpointControllerKey)
	container.ImagePullPolicy = mcoconfig.GetImagePullPolicy(mco.Spec)
	for i, env := range container.Env {
		if env.Name == operatorconfig.PullSecret {
			container.Env[i].Value = mcoconfig.GetImagePullSecret(mco.Spec)
		}
	}
	return container
}

func getImage(mco *mcov1beta2.MultiClusterObservability,
	name, tag, key string,
) string {
	image := mcoconfig.DefaultImgRepository +
		"/" + name + ":" + tag
	found, replacedImage := mcoconfig.ReplaceImage(mco.Annotations, image, key)
	if found {
		return replacedImage
	}
	return image
}
