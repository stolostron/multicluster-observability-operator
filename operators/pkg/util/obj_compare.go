// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/api/equality"

	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"

	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
)

type compFn func(runtime.Object, runtime.Object) bool

var compFns = map[string]compFn{
	"Namespace":                       compareNamespaces,
	"Deployment":                      compareDeployments,
	"ServiceAccount":                  compareServiceAccounts,
	"ClusterRole":                     compareClusterRoles,
	"ClusterRoleBinding":              compareClusterRoleBindings,
	"Secret":                          compareSecrets,
	"Service":                         compareServices,
	"ConfigMap":                       compareConfigMap,
	"CustomResourceDefinition":        compareCRDv1,
	"CustomResourceDefinitionv1":      compareCRDv1,
	"CustomResourceDefinitionv1beta1": compareCRDv1beta1,
	"ObservabilityAddon":              compareObsAddon,
}

func GetK8sObj(kind string) runtime.Object {
	return GetK8sObjWithVersion(kind, "")
}

// GetK8sObj is used to get k8s struct based on the passed-in Kind name
func GetK8sObjWithVersion(kind, version string) runtime.Object {
	objs := map[string]runtime.Object{
		"Namespace":                       &corev1.Namespace{},
		"Deployment":                      &v1.Deployment{},
		"StatefulSet":                     &v1.StatefulSet{},
		"DaemonSet":                       &v1.DaemonSet{},
		"ClusterRole":                     &rbacv1.ClusterRole{},
		"ClusterRoleBinding":              &rbacv1.ClusterRoleBinding{},
		"ServiceAccount":                  &corev1.ServiceAccount{},
		"PersistentVolumeClaim":           &corev1.PersistentVolumeClaim{},
		"Secret":                          &corev1.Secret{},
		"ConfigMap":                       &corev1.ConfigMap{},
		"Service":                         &corev1.Service{},
		"CustomResourceDefinition":        &apiextensionsv1.CustomResourceDefinition{},
		"CustomResourceDefinitionv1":      &apiextensionsv1.CustomResourceDefinition{},
		"CustomResourceDefinitionv1beta1": &apiextensionsv1beta1.CustomResourceDefinition{},
		"ObservabilityAddon":              &mcov1beta1.ObservabilityAddon{},
		"Prometheus":                      &prometheusv1.Prometheus{},
	}
	if kind == "CustomResourceDefinition" {
		kind = kind + version
	}
	return objs[kind]
}

// CompareObject is used to compare two k8s objs are same or not
func CompareObject(re1 runtime.RawExtension, re2 runtime.RawExtension) bool {
	if re2.Object == nil {
		return reflect.DeepEqual(re1.Raw, re2.Raw)
	}
	obj1, err := GetObject(re1)
	if err != nil {
		return false
	}
	obj2, err := GetObject(re2)
	if err != nil {
		return false
	}
	kind1 := obj1.GetObjectKind().GroupVersionKind().Kind
	kind2 := obj2.GetObjectKind().GroupVersionKind().Kind
	version1 := obj1.GetObjectKind().GroupVersionKind().Version
	version2 := obj2.GetObjectKind().GroupVersionKind().Version
	if kind1 != kind2 || version1 != version2 {
		log.Info("obj1 and obj2 have different Kind or Version",
			"kind1", kind2, "kind2", kind2, "version1", version1, "version2", version2)
		return false
	}
	if kind1 == "CustomResourceDefinition" {
		kind1 = kind1 + version1
	}
	return compFns[kind1](obj1, obj2)
}

func GetObject(re runtime.RawExtension) (runtime.Object, error) {
	if re.Object != nil {
		return re.Object, nil
	}
	_, gvk, err := unstructured.UnstructuredJSONScheme.Decode(re.Raw, nil, re.Object)
	if err != nil {
		log.Error(err, "Failed to decode the raw")
		return nil, err
	}
	obj := GetK8sObjWithVersion(gvk.Kind, gvk.Version)
	err = yaml.NewYAMLOrJSONDecoder(strings.NewReader(string(re.Raw)), 100).Decode(obj)
	if err != nil {
		log.Error(err, "Failed to decode the raw to Kind", "kind", gvk.Kind)
		return nil, err
	}
	return obj, nil
}

func compareNamespaces(obj1 runtime.Object, obj2 runtime.Object) bool {
	ns1 := obj1.(*corev1.Namespace)
	ns2 := obj2.(*corev1.Namespace)
	if ns1.Name != ns2.Name {
		log.Info("Find updated namespace in manifestwork", "namespace1", ns1, "namespace2", ns2)
		return false
	}
	return true
}

func compareDeployments(obj1 runtime.Object, obj2 runtime.Object) bool {
	dep1 := obj1.(*v1.Deployment)
	dep2 := obj2.(*v1.Deployment)
	if dep1.Name != dep2.Name || dep1.Namespace != dep2.Namespace {
		log.Info("Find updated name/namespace for deployment", "deployment", dep1.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(dep1.Spec, dep2.Spec) {
		log.Info("Find updated deployment", "deployment", dep1.Name)
		return false
	}
	return true
}

func compareServiceAccounts(obj1 runtime.Object, obj2 runtime.Object) bool {
	sa1 := obj1.(*corev1.ServiceAccount)
	sa2 := obj2.(*corev1.ServiceAccount)
	if sa1.Name != sa2.Name || sa1.Namespace != sa2.Namespace {
		log.Info("Find updated name/namespace for serviceaccount", "serviceaccount", sa1.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(sa1.ImagePullSecrets, sa2.ImagePullSecrets) {
		log.Info("Find updated imagepullsecrets in serviceaccount", "serviceaccount", sa1.Name)
		return false
	}
	return true
}

func compareClusterRoles(obj1 runtime.Object, obj2 runtime.Object) bool {
	cr1 := obj1.(*rbacv1.ClusterRole)
	cr2 := obj2.(*rbacv1.ClusterRole)
	if cr1.Name != cr2.Name {
		log.Info("Find updated name for clusterrole", "clusterrole", cr1.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(cr1.Rules, cr2.Rules) {
		log.Info("Find updated rules in clusterrole", "clusterrole", cr1.Name)
		return false
	}
	return true
}

func compareClusterRoleBindings(obj1 runtime.Object, obj2 runtime.Object) bool {
	crb1 := obj1.(*rbacv1.ClusterRoleBinding)
	crb2 := obj2.(*rbacv1.ClusterRoleBinding)
	if crb1.Name != crb2.Name {
		log.Info("Find updated name/namespace for clusterrolebinding", "clusterrolebinding", crb1.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(crb1.Subjects, crb2.Subjects) || !reflect.DeepEqual(crb1.RoleRef, crb2.RoleRef) {
		log.Info("Find updated subjects/rolerefs for clusterrolebinding", "clusterrolebinding", crb1.Name)
		return false
	}
	return true
}

func compareSecrets(obj1 runtime.Object, obj2 runtime.Object) bool {
	s1 := obj1.(*corev1.Secret)
	s2 := obj2.(*corev1.Secret)
	if s1.Name != s2.Name || s1.Namespace != s2.Namespace {
		log.Info("Find updated name/namespace for secret", "secret", s1.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(s1.Data, s2.Data) {
		log.Info("Find updated data in secret", "secret", s1.Name)
		return false
	}
	return true
}

func compareServices(obj1 runtime.Object, obj2 runtime.Object) bool {
	s1 := obj1.(*corev1.Service)
	s2 := obj2.(*corev1.Service)
	if s1.Name != s2.Name || s1.Namespace != s2.Namespace {
		log.Info("Find updated name/namespace for service", "service", s1.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(s1.Spec, s2.Spec) {
		log.Info("Find updated data in service", "service", s1.Name)
		return false
	}
	return true
}

func compareConfigMap(obj1 runtime.Object, obj2 runtime.Object) bool {
	cm1 := obj1.(*corev1.ConfigMap)
	cm2 := obj2.(*corev1.ConfigMap)
	if cm1.Name != cm2.Name || cm1.Namespace != cm2.Namespace {
		log.Info("Find updated name/namespace for configmap", "configmap", cm1.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(cm1.Data, cm2.Data) {
		log.Info("Find updated data in secret", "secret", cm1.Name)
		return false
	}
	return true
}

func compareCRDv1(obj1 runtime.Object, obj2 runtime.Object) bool {
	crd1 := obj1.(*apiextensionsv1.CustomResourceDefinition)
	crd2 := obj2.(*apiextensionsv1.CustomResourceDefinition)
	if crd1.Name != crd2.Name {
		log.Info("Find updated name for crd", "crd", crd1.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(crd1.Spec, crd2.Spec) {
		log.Info("Find updated spec for crd", "crd", crd1.Name)
		return false
	}
	return true
}

func compareCRDv1beta1(obj1 runtime.Object, obj2 runtime.Object) bool {
	crd1 := obj1.(*apiextensionsv1beta1.CustomResourceDefinition)
	crd2 := obj2.(*apiextensionsv1beta1.CustomResourceDefinition)
	if crd1.Name != crd2.Name {
		log.Info("Find updated name for crd", "crd", crd1.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(crd1.Spec, crd2.Spec) {
		log.Info("Find updated spec for crd", "crd", crd1.Name)
		return false
	}
	return true
}

func compareObsAddon(obj1 runtime.Object, obj2 runtime.Object) bool {
	addon1 := obj1.(*mcov1beta1.ObservabilityAddon)
	addon2 := obj2.(*mcov1beta1.ObservabilityAddon)
	if addon1.Name != addon2.Name || addon1.Namespace != addon2.Namespace {
		log.Info("Find updated name for ObservabilityAddon", "ObservabilityAddon", addon1.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(addon1.Spec, addon2.Spec) {
		log.Info("Find updated spec for ObservabilityAddon", "ObservabilityAddon", addon1.Name)
		return false
	}
	return true
}
