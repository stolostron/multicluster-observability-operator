// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/v3/pkg/resource"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	mcoconfig "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/rendering/templates"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

const (
	deployName         = "endpoint-observability-operator"
	imageName          = "endpoint-monitoring-operator"
	collectorImageName = "metrics-collectore"
	saName             = "endpoint-observability-operator-sa"
	rolebindingName    = "endpoint-observability-operator-rb"
)

var (
	templatePath = "/usr/local/manifests/endpoint-observability"
)

func loadTemplates(namespace string,
	mco *mcov1beta1.MultiClusterObservability) ([]runtime.RawExtension, error) {
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
		obj := util.GetK8sObj(kind)
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(r.Map(), obj)
		if err != nil {
			log.Error(err, "failed to convert the resource", r.GetName())
			return nil, err
		}

		// set the images and watch_namespace for endpoint metrics operator
		if r.GetKind() == "Deployment" && r.GetName() == deployName {
			spec := obj.(*v1.Deployment).Spec.Template.Spec
			if util.GetAnnotation(mco, mcoconfig.AnnotationKeyImageTagSuffix) != "" {
				spec.Containers[0].Image = util.GetAnnotation(mco, mcoconfig.AnnotationKeyImageRepository) +
					"/" +
					imageName + ":" +
					util.GetAnnotation(mco, mcoconfig.AnnotationKeyImageTagSuffix)
			} else {
				spec.Containers[0].Image = mcoconfig.DefaultImgRepository +
					"/" +
					imageName + ":" +
					mcoconfig.EndpointControllerImgTagSuffix
			}
			spec.Containers[0].ImagePullPolicy = mco.Spec.ImagePullPolicy
			for i, env := range spec.Containers[0].Env {
				if env.Name == "WATCH_NAMESPACE" {
					spec.Containers[0].Env[i].Value = namespace
				}
				if env.Name == "COLLECTOR_IMAGE" {
					if util.GetAnnotation(mco, mcoconfig.AnnotationKeyImageTagSuffix) != "" {
						spec.Containers[0].Env[i].Value = util.GetAnnotation(mco, mcoconfig.AnnotationKeyImageRepository) +
							"/" +
							collectorImageName + ":" +
							util.GetAnnotation(mco, mcoconfig.AnnotationKeyImageTagSuffix)
					} else {
						spec.Containers[0].Image = mcoconfig.DefaultImgRepository +
							"/" +
							imageName + ":" +
							mcoconfig.MetricsCollectorImgTagSuffix
					}
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
		rawExtensionList = append(rawExtensionList, runtime.RawExtension{Object: obj})
	}
	return rawExtensionList, nil
}
