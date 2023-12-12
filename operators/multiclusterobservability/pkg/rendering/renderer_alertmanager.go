// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"context"
	"fmt"
	"strconv"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/kustomize/api/resource"

	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
)

func (r *MCORenderer) newAlertManagerRenderer() {
	r.renderAlertManagerFns = map[string]rendererutil.RenderFn{
		"StatefulSet":           r.renderAlertManagerStatefulSet,
		"Service":               r.renderer.RenderNamespace,
		"ServiceAccount":        r.renderer.RenderNamespace,
		"ConfigMap":             r.renderAlertManagerConfigMap,
		"ClusterRole":           r.renderer.RenderClusterRole,
		"ClusterRoleBinding":    r.renderer.RenderClusterRoleBinding,
		"Secret":                r.renderAlertManagerSecret,
		"Role":                  r.renderer.RenderNamespace,
		"RoleBinding":           r.renderer.RenderNamespace,
		"Ingress":               r.renderer.RenderNamespace,
		"PersistentVolumeClaim": r.renderer.RenderNamespace,
		"ServiceMonitor":        r.renderer.RenderNamespace,
		"PrometheusRule":        r.renderer.RenderNamespace,
	}
}

func (r *MCORenderer) renderAlertManagerStatefulSet(res *resource.Resource,
	namespace string, labels map[string]string) (*unstructured.Unstructured, error) {
	u, err := r.renderer.RenderNamespace(res, namespace, labels)
	if err != nil {
		return nil, err
	}
	obj := util.GetK8sObj(u.GetKind())
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj)
	if err != nil {
		return nil, err
	}
	crLabelKey := mcoconfig.GetCrLabelKey()
	dep := obj.(*v1.StatefulSet)
	dep.ObjectMeta.Labels[crLabelKey] = r.cr.Name
	dep.Spec.Selector.MatchLabels[crLabelKey] = r.cr.Name
	dep.Spec.Template.ObjectMeta.Labels[crLabelKey] = r.cr.Name
	dep.Name = mcoconfig.GetOperandName(mcoconfig.Alertmanager)
	dep.Spec.Replicas = mcoconfig.GetReplicas(mcoconfig.Alertmanager, r.cr.Spec.ReadTShirtSize, r.cr.Spec.AdvancedConfig)

	spec := &dep.Spec.Template.Spec

	imagePullPolicy := mcoconfig.GetImagePullPolicy(r.cr.Spec)
	spec.Containers[0].ImagePullPolicy = imagePullPolicy
	args := spec.Containers[0].Args

	if *dep.Spec.Replicas > 1 {
		for i := int32(0); i < *dep.Spec.Replicas; i++ {
			args = append(args, "--cluster.peer="+
				mcoconfig.GetOperandName(mcoconfig.Alertmanager)+"-"+
				strconv.Itoa(int(i))+".alertmanager-operated."+
				mcoconfig.GetDefaultNamespace()+".svc:9094")
		}
	}

	spec.Containers[0].Args = args
	spec.Containers[0].Resources = mcoconfig.GetResources(mcoconfig.Alertmanager, r.cr.Spec.ReadTShirtSize, r.cr.Spec.AdvancedConfig)

	spec.Containers[1].ImagePullPolicy = imagePullPolicy
	spec.NodeSelector = r.cr.Spec.NodeSelector
	spec.Tolerations = r.cr.Spec.Tolerations
	spec.ImagePullSecrets = []corev1.LocalObjectReference{
		{Name: mcoconfig.GetImagePullSecret(r.cr.Spec)},
	}

	spec.Containers[0].Image = mcoconfig.DefaultImgRepository + "/" + mcoconfig.AlertManagerImgName +
		":" + mcoconfig.DefaultImgTagSuffix
	//replace the alertmanager and config-reloader images
	found, image := mcoconfig.ReplaceImage(
		r.cr.Annotations,
		mcoconfig.DefaultImgRepository+"/"+mcoconfig.AlertManagerImgName,
		mcoconfig.AlertManagerImgKey)
	if found {
		spec.Containers[0].Image = image
	}

	found, image = mcoconfig.ReplaceImage(r.cr.Annotations, mcoconfig.ConfigmapReloaderImgRepo,
		mcoconfig.ConfigmapReloaderKey)
	if found {
		spec.Containers[1].Image = image
	}
	// the oauth-proxy image only exists in mch-image-manifest configmap
	// pass nil annotation to make sure oauth-proxy overrided from mch-image-manifest
	found, image = mcoconfig.ReplaceImage(nil, mcoconfig.OauthProxyImgRepo,
		mcoconfig.OauthProxyKey)
	if found {
		spec.Containers[2].Image = image
	}
	spec.Containers[2].ImagePullPolicy = imagePullPolicy

	// fail if kube-rbac-proxy container is not at the expected index
	if spec.Containers[3].Name != "kube-rbac-proxy" {
		return nil, fmt.Errorf("kube-rbac-proxy container not found in statefulset")
	}
	if ok, image := mcoconfig.ReplaceImage(r.cr.Annotations, mcoconfig.DefaultImgRepository+"/"+mcoconfig.KubeRBACProxyImgName, mcoconfig.KubeRBACProxyKey); ok {
		spec.Containers[3].Image = image
	}
	spec.Containers[3].ImagePullPolicy = imagePullPolicy

	//replace the volumeClaimTemplate
	dep.Spec.VolumeClaimTemplates[0].Spec.StorageClassName = &r.cr.Spec.StorageConfig.StorageClass
	dep.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage] =
		apiresource.MustParse(r.cr.Spec.StorageConfig.AlertmanagerStorageSize)

	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	return &unstructured.Unstructured{Object: unstructuredObj}, nil
}

func (r *MCORenderer) renderAlertManagerSecret(res *resource.Resource,
	namespace string, labels map[string]string) (*unstructured.Unstructured, error) {
	u, err := r.renderer.RenderNamespace(res, namespace, labels)
	if err != nil {
		return nil, err
	}

	if u.GetName() == "alertmanager-proxy" {
		obj := util.GetK8sObj(u.GetKind())
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj)
		if err != nil {
			return nil, err
		}
		srt := obj.(*corev1.Secret)
		p, err := util.GeneratePassword(43)
		if err != nil {
			return nil, err
		}
		srt.Data["session_secret"] = []byte(p)
		unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return nil, err
		}
		return &unstructured.Unstructured{Object: unstructuredObj}, nil
	}

	return u, nil
}

func (r *MCORenderer) renderAlertManagerConfigMap(res *resource.Resource,
	namespace string, labels map[string]string) (*unstructured.Unstructured, error) {
	u, err := r.renderer.RenderNamespace(res, namespace, labels)
	if err != nil {
		return nil, err
	}

	if u.GetName() == "alertmanager-clientca-metric" {
		cm := &corev1.ConfigMap{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, cm)
		if err != nil {
			return nil, fmt.Errorf("failed to convert %q to ConfigMap: %w", u.GetName(), err)
		}

		// Retrieve the extension-apiserver-authentication ConfigMap from kube-system namespace
		namespacedName := types.NamespacedName{
			Name:      "extension-apiserver-authentication",
			Namespace: "kube-system",
		}
		sourceConfigMap := &corev1.ConfigMap{}
		err = r.kubeClient.Get(context.Background(), namespacedName, sourceConfigMap)
		if err != nil {
			return nil, fmt.Errorf("error fetching source ConfigMap: %w", err)
		}

		// Extract the CA certificate data
		caData, exists := sourceConfigMap.Data["client-ca-file"]
		if !exists {
			return nil, fmt.Errorf("client-ca-file not found in source ConfigMap")
		}

		if len(caData) == 0 {
			return nil, fmt.Errorf("client-ca-file is empty in source ConfigMap")
		}

		// Update the ConfigMap with the CA certificate data
		cm.Data["client-ca-file"] = caData

		unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cm)
		if err != nil {
			return nil, err
		}

		return &unstructured.Unstructured{Object: unstructuredObj}, nil
	}

	return u, nil
}

func (r *MCORenderer) renderAlertManagerTemplates(templates []*resource.Resource,
	namespace string, labels map[string]string) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderAlertManagerFns[template.GetKind()]
		if !ok {
			m, err := template.Map()
			if err != nil {
				return []*unstructured.Unstructured{}, err
			}
			uobjs = append(uobjs, &unstructured.Unstructured{Object: m})
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
