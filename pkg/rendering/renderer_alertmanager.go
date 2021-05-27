// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"strconv"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/v3/pkg/resource"

	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/util"
)

func (r *Renderer) newAlertManagerRenderer() {
	r.renderAlertManagerFns = map[string]renderFn{
		"StatefulSet":           r.renderAlertManagerStatefulSet,
		"Service":               r.renderNamespace,
		"ServiceAccount":        r.renderNamespace,
		"ConfigMap":             r.renderNamespace,
		"ClusterRole":           r.renderClusterRole,
		"ClusterRoleBinding":    r.renderClusterRoleBinding,
		"Secret":                r.renderNamespace,
		"Role":                  r.renderNamespace,
		"RoleBinding":           r.renderNamespace,
		"Ingress":               r.renderNamespace,
		"PersistentVolumeClaim": r.renderNamespace,
	}
}

func (r *Renderer) renderAlertManagerStatefulSet(res *resource.Resource) (*unstructured.Unstructured, error) {
	u, err := r.renderDeployments(res)
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
	dep.Name = mcoconfig.GetObjectPrefix() + "-" + dep.Name
	dep.Spec.Replicas = mcoconfig.GetReplicas(mcoconfig.Alertmanager, r.cr.Spec.AdvancedConfig)

	spec := &dep.Spec.Template.Spec
	spec.Containers[0].ImagePullPolicy = mcoconfig.GetImagePullPolicy(r.cr.Spec)
	args := spec.Containers[0].Args

	if *dep.Spec.Replicas > 1 {
		for i := int32(0); i < *dep.Spec.Replicas; i++ {
			args = append(args, "--cluster.peer="+
				mcoconfig.GetObjectPrefix()+"-alertmanager-"+
				strconv.Itoa(int(i))+".alertmanager-operated."+
				mcoconfig.GetDefaultNamespace()+".svc:9094")
		}
	}

	spec.Containers[0].Args = args
	spec.Containers[0].Resources = mcoconfig.GetResources(mcoconfig.Alertmanager, r.cr.Spec.AdvancedConfig)

	spec.Containers[1].ImagePullPolicy = mcoconfig.GetImagePullPolicy(r.cr.Spec)
	spec.NodeSelector = r.cr.Spec.NodeSelector
	spec.Tolerations = r.cr.Spec.Tolerations
	spec.ImagePullSecrets = []corev1.LocalObjectReference{
		{Name: mcoconfig.GetImagePullSecret(r.cr.Spec)},
	}

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

func (r *Renderer) renderAlertManagerTemplates(templates []*resource.Resource) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderAlertManagerFns[template.GetKind()]
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
